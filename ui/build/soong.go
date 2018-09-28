// Copyright 2017 Google Inc. All rights reserved.
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

package build

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/blueprint/microfactory"

	"android/soong/ui/status"
)

func runSoong(ctx Context, config Config) {
	ctx.BeginTrace("soong")
	defer ctx.EndTrace()

	func() {
		ctx.BeginTrace("blueprint bootstrap")
		defer ctx.EndTrace()

		cmd := Command(ctx, config, "blueprint bootstrap", "build/blueprint/bootstrap.bash", "-t")
		cmd.Environment.Set("BLUEPRINTDIR", "./build/blueprint")
		cmd.Environment.Set("BOOTSTRAP", "./build/blueprint/bootstrap.bash")
		cmd.Environment.Set("BUILDDIR", config.SoongOutDir())
		cmd.Environment.Set("GOROOT", "./"+filepath.Join("prebuilts/go", config.HostPrebuiltTag()))
		cmd.Environment.Set("BLUEPRINT_LIST_FILE", filepath.Join(config.FileListDir(), "Android.bp.list"))
		cmd.Environment.Set("NINJA_BUILDDIR", config.OutDir())
		cmd.Environment.Set("SRCDIR", ".")
		cmd.Environment.Set("TOPNAME", "Android.bp")
		cmd.Sandbox = soongSandbox

		cmd.RunAndPrintOrFatal()
	}()

	func() {
		ctx.BeginTrace("environment check")
		defer ctx.EndTrace()

		envFile := filepath.Join(config.SoongOutDir(), ".soong.environment")
		envTool := filepath.Join(config.SoongOutDir(), ".bootstrap/bin/soong_env")
		if _, err := os.Stat(envFile); err == nil {
			if _, err := os.Stat(envTool); err == nil {
				cmd := Command(ctx, config, "soong_env", envTool, envFile)
				cmd.Sandbox = soongSandbox

				var buf strings.Builder
				cmd.Stdout = &buf
				cmd.Stderr = &buf
				if err := cmd.Run(); err != nil {
					ctx.Verboseln("soong_env failed, forcing manifest regeneration")
					os.Remove(envFile)
				}

				if buf.Len() > 0 {
					ctx.Verboseln(buf.String())
				}
			} else {
				ctx.Verboseln("Missing soong_env tool, forcing manifest regeneration")
				os.Remove(envFile)
			}
		} else if !os.IsNotExist(err) {
			ctx.Fatalf("Failed to stat %f: %v", envFile, err)
		}
	}()

	var cfg microfactory.Config
	cfg.Map("github.com/google/blueprint", "build/blueprint")

	cfg.TrimPath = absPath(ctx, ".")

	func() {
		ctx.BeginTrace("minibp")
		defer ctx.EndTrace()

		minibp := filepath.Join(config.SoongOutDir(), ".minibootstrap/minibp")
		if _, err := microfactory.Build(&cfg, minibp, "github.com/google/blueprint/bootstrap/minibp"); err != nil {
			ctx.Fatalln("Failed to build minibp:", err)
		}
	}()

	func() {
		ctx.BeginTrace("bpglob")
		defer ctx.EndTrace()

		bpglob := filepath.Join(config.SoongOutDir(), ".minibootstrap/bpglob")
		if _, err := microfactory.Build(&cfg, bpglob, "github.com/google/blueprint/bootstrap/bpglob"); err != nil {
			ctx.Fatalln("Failed to build bpglob:", err)
		}
	}()

	func() {
		ctx.BeginTrace("QSSI_violators")
		defer ctx.EndTrace()

		cmd := Command(ctx, config, config.PrebuiltBuildTool("qssi"),
			"vendor/qcom/opensource/core-utils/build/QSSI_violators")

		cmd.Sandbox = soongSandbox
		err := cmd.Run()
		if err != nil {
			ctx.Verboseln("QSSI_violators returned error...")
		}
	}()

	ninja := func(name, file string) {
		ctx.BeginTrace(name)
		defer ctx.EndTrace()

		fifo := filepath.Join(config.OutDir(), ".ninja_fifo")
		status.NinjaReader(ctx, ctx.Status.StartTool(), fifo)

		cmd := Command(ctx, config, "soong "+name,
			config.PrebuiltBuildTool("ninja"),
			"-d", "keepdepfile",
			"-w", "dupbuild=err",
			"-j", strconv.Itoa(config.Parallel()),
			"--frontend_file", fifo,
			"-f", filepath.Join(config.SoongOutDir(), file))
		cmd.Sandbox = soongSandbox
		cmd.RunAndPrintOrFatal()
	}

	ninja("minibootstrap", ".minibootstrap/build.ninja")
	ninja("bootstrap", ".bootstrap/build.ninja")
}
