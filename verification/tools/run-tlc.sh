#!/bin/sh
# Run TLC on one TLA+ model with the pinned tla2tools release.
#
# Usage: verification/tools/run-tlc.sh <model-dir> <Module>
#   Reads <model-dir>/<Module>.tla and <model-dir>/<Module>.cfg.
#   Exit 0: every invariant and property holds on the model's bounds.
#   Non-zero: TLC reports a violation, a deadlock, or a model error.
#
# Tool: TLA+ tla2tools 1.7.4, SHA-256 pinned below. Set TLA2TOOLS to a jar to
# use another copy. Java 11 or later must be on PATH.
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: $0 <model-dir> <Module>" >&2
    exit 2
fi

MODEL_DIR=$1
MODULE=$2
JAR_URL=https://github.com/tlaplus/tlaplus/releases/download/v1.7.4/tla2tools.jar
JAR_SHA256=936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88
CACHE=${TLC_CACHE:-$HOME/.cache/dynamis-tlc}
JAR=${TLA2TOOLS:-$CACHE/tla2tools-1.7.4.jar}

if [ -z "${TLA2TOOLS:-}" ] && [ ! -f "$JAR" ]; then
    mkdir -p "$CACHE"
    curl -fsSL -o "$JAR.part" "$JAR_URL"
    echo "$JAR_SHA256  $JAR.part" | shasum -a 256 -c - >/dev/null || {
        rm -f "$JAR.part"
        echo "tla2tools checksum mismatch; refusing to use $JAR_URL" >&2
        exit 1
    }
    mv "$JAR.part" "$JAR"
fi

cd "$MODEL_DIR"
# TLC state files go to a temp directory, so the repo never gains a states/ folder.
META=$(mktemp -d "${TMPDIR:-/tmp}/tlc-meta.XXXXXX")
set +e
java -XX:+UseParallelGC -cp "$JAR" tlc2.TLC -workers auto -metadir "$META" -config "$MODULE.cfg" "$MODULE.tla"
STATUS=$?
set -e
rm -rf "$META"
exit "$STATUS"
