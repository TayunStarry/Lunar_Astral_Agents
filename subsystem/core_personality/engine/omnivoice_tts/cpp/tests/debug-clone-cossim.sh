#!/bin/bash

for backend in CUDA0 Vulkan0 CPU; do
    for quant in F32 BF16 Q8_0 Q4_K_M; do
        GGML_BACKEND=$backend ./debug-clone-cossim.py --quant $quant \
            2>&1 | tee clone-${backend}-${quant}.log
    done
done
