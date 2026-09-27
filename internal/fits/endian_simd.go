//go:build goexperiment.simd

package fits

import (
	"simd"
	"unsafe"
)

func reorderU16(dst, src []byte) {
	if !canVector(dst, src, 2) {
		reorderU16Scalar(dst, src)
		return
	}
	dst16 := unsafe.Slice((*uint16)(unsafe.Pointer(&dst[0])), len(dst)/2)
	src16 := unsafe.Slice((*uint16)(unsafe.Pointer(&src[0])), len(src)/2)
	var width simd.Uint16s
	n := width.Len()
	mask := simd.BroadcastUint16s(0x8000)
	i := 0
	for ; i+n <= len(src16); i += n {
		u := simd.LoadUint16s(src16[i : i+n]).Xor(mask).RotateAllLeft(8)
		u.Store(dst16[i : i+n])
	}
	if i < len(src16) {
		u, _ := simd.LoadUint16sPart(src16[i:])
		u = u.Xor(mask).RotateAllLeft(8)
		u.StorePart(dst16[i:])
	}
}

func reorderU32(dst, src []byte) {
	if !canVector(dst, src, 4) {
		reorderU32Scalar(dst, src)
		return
	}
	dst32 := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), len(dst)/4)
	src32 := unsafe.Slice((*uint32)(unsafe.Pointer(&src[0])), len(src)/4)
	var width simd.Uint32s
	n := width.Len()
	sign := simd.BroadcastUint32s(0x80000000)
	low := simd.BroadcastUint32s(0x00FF00FF)
	high := simd.BroadcastUint32s(0xFF00FF00)
	i := 0
	for ; i+n <= len(src32); i += n {
		u := byteSwap32(simd.LoadUint32s(src32[i:i+n]).Xor(sign), low, high)
		u.Store(dst32[i : i+n])
	}
	if i < len(src32) {
		u, _ := simd.LoadUint32sPart(src32[i:])
		u = byteSwap32(u.Xor(sign), low, high)
		u.StorePart(dst32[i:])
	}
}

func reorderF32(dst, src []byte) {
	if !canVector(dst, src, 4) {
		reorderF32Scalar(dst, src)
		return
	}
	dst32 := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), len(dst)/4)
	src32 := unsafe.Slice((*uint32)(unsafe.Pointer(&src[0])), len(src)/4)
	var width simd.Uint32s
	n := width.Len()
	low := simd.BroadcastUint32s(0x00FF00FF)
	high := simd.BroadcastUint32s(0xFF00FF00)
	i := 0
	for ; i+n <= len(src32); i += n {
		u := byteSwap32(simd.LoadUint32s(src32[i:i+n]), low, high)
		u.Store(dst32[i : i+n])
	}
	if i < len(src32) {
		u, _ := simd.LoadUint32sPart(src32[i:])
		u = byteSwap32(u, low, high)
		u.StorePart(dst32[i:])
	}
}

func reorderF64(dst, src []byte) {
	if !canVector(dst, src, 8) {
		reorderF64Scalar(dst, src)
		return
	}
	dst64 := unsafe.Slice((*uint64)(unsafe.Pointer(&dst[0])), len(dst)/8)
	src64 := unsafe.Slice((*uint64)(unsafe.Pointer(&src[0])), len(src)/8)
	var width simd.Uint64s
	n := width.Len()
	i := 0
	for ; i+n <= len(src64); i += n {
		u := byteSwap64(simd.LoadUint64s(src64[i : i+n]))
		u.Store(dst64[i : i+n])
	}
	if i < len(src64) {
		u, _ := simd.LoadUint64sPart(src64[i:])
		u = byteSwap64(u)
		u.StorePart(dst64[i:])
	}
}

// byteSwap32 reverses the bytes of each lane. The host is little-endian, so
// storing the swapped value writes big-endian FITS bytes.
func byteSwap32(x, low, high simd.Uint32s) simd.Uint32s {
	x = x.RotateAllLeft(16)
	return x.And(low).ShiftAllLeft(8).Or(x.And(high).ShiftAllRight(8))
}

func byteSwap64(x simd.Uint64s) simd.Uint64s {
	x = x.RotateAllLeft(32)
	x = x.And(simd.BroadcastUint64s(0x0000FFFF0000FFFF)).ShiftAllLeft(16).Or(
		x.And(simd.BroadcastUint64s(0xFFFF0000FFFF0000)).ShiftAllRight(16))
	return x.And(simd.BroadcastUint64s(0x00FF00FF00FF00FF)).ShiftAllLeft(8).Or(
		x.And(simd.BroadcastUint64s(0xFF00FF00FF00FF00)).ShiftAllRight(8))
}

func canVector(dst, src []byte, align uintptr) bool {
	if len(src) == 0 || len(dst) < len(src) {
		return false
	}
	var probe uint16 = 1
	if *(*byte)(unsafe.Pointer(&probe)) != 1 {
		return false
	}
	return uintptr(unsafe.Pointer(&src[0]))%align == 0 && uintptr(unsafe.Pointer(&dst[0]))%align == 0
}
