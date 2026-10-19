# Replace tf2onnx's unconverted TFL_GELU (exact/erf form) with 0.5*x*(1+erf(x/sqrt2)).
import sys, math, onnx
from onnx import helper, numpy_helper
import numpy as np

src, dst = sys.argv[1], sys.argv[2]
m = onnx.load(src)
g = m.graph
consts = {
    "gelu_inv_sqrt2": 1 / math.sqrt(2),
    "gelu_one": 1.0,
    "gelu_half": 0.5,
}
for k, v in consts.items():
    g.initializer.append(numpy_helper.from_array(np.array(v, dtype=np.float32), k))

new = []
n_gelu = 0
for node in g.node:
    if node.op_type != "TFL_GELU":
        new.append(node)
        continue
    n_gelu += 1
    x, y, p = node.input[0], node.output[0], f"gelu{n_gelu}_"
    new += [
        helper.make_node("Mul", [x, "gelu_inv_sqrt2"], [p + "s"]),
        helper.make_node("Erf", [p + "s"], [p + "e"]),
        helper.make_node("Add", [p + "e", "gelu_one"], [p + "a"]),
        helper.make_node("Mul", [x, p + "a"], [p + "m"]),
        helper.make_node("Mul", [p + "m", "gelu_half"], [y]),
    ]
del g.node[:]
g.node.extend(new)
keep = [o for o in m.opset_import if o.domain in ("", "ai.onnx")]
del m.opset_import[:]
m.opset_import.extend(keep)
onnx.checker.check_model(m, full_check=True)
onnx.save(m, dst)
print("replaced GELU:", n_gelu, "->", dst)
