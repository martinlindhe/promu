// Copyright © 2016 Prometheus Team
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	shell "github.com/progrium/go-shell"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/prometheus/promu/util/sh"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build a Go project",
	Long:  `Build a Go project`,
	Run: func(cmd *cobra.Command, args []string) {
		runBuild()
	},
	PreRun: func(cmd *cobra.Command, args []string) {
		if err := hasRequiredConfigurations("repository.path"); err != nil {
			fatal(err)
		}
	},
}

// init prepares cobra flags
func init() {
	Promu.AddCommand(buildCmd)

	buildCmd.Flags().Bool("cgo", false, "Enable CGO")
	buildCmd.Flags().String("prefix", "", "Specific dir to store binaries (default is .)")

	viper.BindPFlag("build.prefix", buildCmd.Flags().Lookup("prefix"))
	viper.BindPFlag("go.cgo", buildCmd.Flags().Lookup("cgo"))
}

// Binary represents a built binary.
type Binary struct {
	Name string
	Path string
}

func runBuild() {
	defer shell.ErrExit()
	shell.Tee = os.Stdout

	if viper.GetBool("verbose") {
		shell.Trace = true
	}

	var (
		cgo      = viper.GetBool("go.cgo")
		prefix   = viper.GetString("build.prefix")
		repoPath = viper.GetString("repository.path")
		flags    = viper.GetString("build.flags")

		ext      string
		binaries []Binary
		ldflags  string
	)

	if goos == "windows" {
		ext = ".exe"
	}

	ldflags = getLdflags(info)

	os.Setenv("GO15VENDOREXPERIMENT", "1")
	if cgo {
		os.Setenv("CGO_ENABLED", "1")
		defer os.Unsetenv("CGO_ENABLED")
	}

	if err := viper.UnmarshalKey("build.binaries", &binaries); err != nil {
		fatalMsg("Failed to Unmashal binaries", err)
	}

	for _, binary := range binaries {
		binaryName := fmt.Sprintf("%s%s", binary.Name, ext)
		fmt.Printf(" >   %s\n", binaryName)

		params := []string{"build",
			"-o", path.Join(prefix, binaryName),
			"-ldflags", ldflags,
		}
		params = append(params, sh.SplitParameters(flags)...)
		params = append(params, path.Join(repoPath, binary.Path))
		err := sh.RunCommand("go", params...)
		if err != nil {
			fmt.Println("command failed:", err)
		}
	}
}

func getLdflags(info ProjectInfo) string {
	var ldflags []string

	if viper.IsSet("build.ldflags") {
		var (
			tmplOutput = new(bytes.Buffer)
			fnMap      = template.FuncMap{
				"date":     time.Now().UTC().Format,
				"host":     os.Hostname,
				"repoPath": RepoPathFunc,
				"user":     UserFunc,
			}
			ldflagsTmpl = viper.GetString("build.ldflags")
		)

		tmpl, err := template.New("ldflags").Funcs(fnMap).Parse(ldflagsTmpl)
		if err != nil {
			fatalMsg("Failed to parse ldflags text/template", err)
		}

		if err := tmpl.Execute(tmplOutput, info); err != nil {
			fatalMsg("Failed to execute ldflags text/template", err)
		}

		ldflags = append(ldflags, strings.Split(tmplOutput.String(), "\n")...)
	} else {
		ldflags = append(ldflags, fmt.Sprintf("-X main.Version=%s", info.Version))
	}

	if goos == "darwin" {
		if !stringInSlice("-s", ldflags) {
			// Fixes dwarf error: golang/go#11994
			ldflags = append(ldflags, "-s")
		}
	} else {
		if !stringInSlice(`-extldflags '-static'`, ldflags) {
			ldflags = append(ldflags, `-extldflags '-static'`)
		}
	}

	return strings.Join(ldflags[:], " ")
}

// UserFunc returns the current username.
func UserFunc() (interface{}, error) {
	// os/user.Current() doesn't always work without CGO
	return shellOutput("whoami"), nil
}

// RepoPathFunc returns the repository path.
func RepoPathFunc() interface{} {
	return viper.GetString("repository.path")
}
