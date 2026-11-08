"""Make an isolated checkout for real dango checks, without editing the source."""
import os
from pathlib import Path
import shutil
import sys

source, destination = map(Path, sys.argv[1:])
# Generated output and local configuration never belong in this fixture.
excluded = {".git", ".cache", ".agents", ".codex", ".aws", "bin", "build", "dist",
            "node_modules", "public", "resources", "__pycache__", ".atago"}


def link_or_copy(src, dst):
    try:
        os.link(src, dst)
    except OSError:
        shutil.copy2(src, dst)


shutil.copytree(source, destination,
                ignore=lambda directory, names: excluded.intersection(names),
                copy_function=link_or_copy)
# Decision checks must also work without the narrative ADR documents.
shutil.rmtree(destination / "docs/decisions")
# Add a new file only: linked source files must never be modified.
probe = destination / "internal/layer1/dangofixture"
probe.mkdir()
(probe / "fixture.go").write_text('package dangofixture\n\nimport "io/fs"\n\ntype Option func(*int)\nfunc LoadWrong(fsys fs.FS, path string) (string, error) { return "", nil }\n')

(destination / "cmd/gozen/dango_fixture.go").write_text(
    'package main\n\nfunc dangoFixture(c *command) {\n'
    'var cfg scriptConfig\n'
    '_ = c.parse(scriptFlags(&cfg), []string{"--seed", "1"}, pages["sim script"])\n'
    '}\n')
