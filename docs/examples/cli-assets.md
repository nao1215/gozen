# CLI fixture の素材台帳

`e2e/atago/fixtures/game` の YAML、名前、文章は T4 用に新規作成した一般向けのキャンプ探索です。`assets/mote.png` はコードで描いた 8×8 の単色の目印で、Gozen の MIT ライセンスです。

再生成のソースは [cli-fixtures.go.txt](cli-fixtures.go.txt)。リポジトリルートで拡張子を `.go` にして一時ディレクトリへコピーし、`go run <一時ファイル>` を実行すると、この fixture の PNG とフォントを書き直します。出力先はリポジトリ内の固定の fixture だけです。

`assets/regular.ttf` は `golang.org/x/image v0.45.0/font/gofont/goregular` の Go Regular を変更せずコピーしたものです。作者は Bigelow & Holmes、出典は [Go fonts](https://go.googlesource.com/image/+/refs/tags/v0.45.0/font/gofont/ttfs/README)。公開 fixture と一緒に次の通知を提供します。バイナリのテスト生成物を配布するときも通知を添付してください。

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
