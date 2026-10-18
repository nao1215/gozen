# Asset sources and licenses

The sample games use original, general-audience content created for Gozen. No artwork, writing, sound, screenshots or recordings from another game are included.

| Material | Source / reproduction | License |
| --- | --- | --- |
| Wayfinder backgrounds, Pebble and Courier sprites | Geometric pixel art drawn at runtime by `examples/play/art.go`; no external image input | [MIT](LICENSE) |
| Wayfinder title, characters, dialogue, maps and rules | `examples/play/game/*.yaml`, authored for this repository | MIT |
| Minimal and headless examples | `examples/minimal/` and `examples/headless/`; headless picture generated in Go | MIT |
| Showcase PNG and GIF | Actual Wayfinder Draw-loop captures; regenerate with `scripts/capture-example.sh` under Xvfb, as described in [the sample guide](docs/examples/wayfinder.md#capture) | MIT for original art and writing; the font notice below also applies |
| Go Regular font embedded at runtime in Wayfinder | `golang.org/x/image v0.45.0/font/gofont/goregular`, Bigelow & Holmes; [upstream notice](https://go.googlesource.com/image/+/refs/tags/v0.45.0/font/gofont/ttfs/README) | BSD notice below |
| CLI synthetic pattern images and catalog | `docs/examples/production/generate_fixture.py` and neighboring original fixtures; [details](docs/development/production-tools.md#合成画像での実行) | MIT |

No music, recorded audio, image generation models or ONNX models are bundled with the sample. Go Regular is obtained through the Go dependency and included in the compiled example, rather than copied into its source directory. Include this notice when redistributing the example binary or its captures. Other CLI/test font fixtures have their notices in [CLI assets](docs/examples/cli-assets.md).

## Go font notice

```text
These fonts were created by the Bigelow & Holmes foundry specifically for the
Go project. See https://blog.golang.org/go-fonts for details.

They are licensed under the same open source license as the rest of the Go
project's software:

Copyright (c) 2016 Bigelow & Holmes Inc.. All rights reserved.

Distribution of this font is governed by the following license. If you do not
agree to this license, including the disclaimer, do not distribute or modify
this font.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

	* Redistributions of source code must retain the above copyright notice,
	  this list of conditions and the following disclaimer.

	* Redistributions in binary form must reproduce the above copyright notice,
	  this list of conditions and the following disclaimer in the documentation
	  and/or other materials provided with the distribution.

	* Neither the name of Google Inc. nor the names of its contributors may be
	  used to endorse or promote products derived from this software without
	  specific prior written permission.

DISCLAIMER: THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```
