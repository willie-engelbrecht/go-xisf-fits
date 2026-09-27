//go:build !goexperiment.simd

package fits

func reorderU16(dst, src []byte) { reorderU16Scalar(dst, src) }
func reorderU32(dst, src []byte) { reorderU32Scalar(dst, src) }
func reorderF32(dst, src []byte) { reorderF32Scalar(dst, src) }
func reorderF64(dst, src []byte) { reorderF64Scalar(dst, src) }
