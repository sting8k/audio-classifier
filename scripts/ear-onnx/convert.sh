#!/usr/bin/env bash
# Rebuild ear.onnx (the app's language model) from Desert Ant Labs' ear.tflite.
# The result is published as a GitHub release asset and pinned by sha256 in scripts/fetch-assets.sh.
#
#   scripts/ear-onnx/convert.sh [workdir]     # needs python3.12 and uv (or python3 -m venv + pip)
#
# tf2onnx 1.17.0 predates the LiteRT flatbuffer this model uses; tf2onnx-1.17.0.patch fixes:
#   - opcodes read only from DeprecatedBuiltinCode (new files keep the real code in BuiltinCode),
#   - constants stored by buffer offset/size instead of inline data,
#   - FULLY_CONNECTED on rank-3 input dropping its bias (silent: converts, then predicts garbage),
#   - small guards the above used to trip.
# TFL_GELU is not supported by tf2onnx; fix_gelu.py rewrites it as exact (erf) GELU.
# Node order (so the sha256) can differ between runs; outputs are identical. Check any rebuilt
# file against the tflite on real clips before publishing it.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
W=${1:-$(mktemp -d)}
REV=aca9062c7e27f290306df0d65cd781d8047e07b1   # desert-ant-labs/ear
TFLITE_SHA=a19b43892abe53afba50122a861b10fd76f312a5027a5c659b0738081135feeb
mkdir -p "$W"; cd "$W"
[ -f ear.tflite ] || curl -fsSL -o ear.tflite "https://huggingface.co/desert-ant-labs/ear/resolve/$REV/ear.tflite"
echo "$TFLITE_SHA  ear.tflite" | sha256sum -c -
if command -v uv >/dev/null; then
  uv venv -q -p 3.12 venv
  uv pip install -q --link-mode=copy --python venv/bin/python tf2onnx==1.17.0 tensorflow-cpu==2.21.0 onnx==1.23.2 numpy==2.5.4 protobuf==7.36.2 flatbuffers==25.12.19
else
  python3.12 -m venv venv
  venv/bin/pip install -q tf2onnx==1.17.0 tensorflow-cpu==2.21.0 onnx==1.23.2 numpy==2.5.4 protobuf==7.36.2 flatbuffers==25.12.19
fi
SP=$(venv/bin/python -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')
patch -d "$SP" -p1 < "$HERE/tf2onnx-1.17.0.patch"
venv/bin/python -m tf2onnx.convert --tflite ear.tflite --output ear_tf2onnx.onnx --opset 17
venv/bin/python "$HERE/fix_gelu.py" ear_tf2onnx.onnx ear.onnx
sha256sum ear.onnx
