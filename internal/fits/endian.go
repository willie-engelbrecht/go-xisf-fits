package fits

import "encoding/binary"

func reorderU16Scalar(dst, src []byte) {
	for i := 0; i < len(src); i += 2 {
		v := binary.LittleEndian.Uint16(src[i:])
		binary.BigEndian.PutUint16(dst[i:], v^0x8000)
	}
}

func reorderU32Scalar(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		v := binary.LittleEndian.Uint32(src[i:])
		binary.BigEndian.PutUint32(dst[i:], v^0x80000000)
	}
}

func reorderF32Scalar(dst, src []byte) {
	for i := 0; i < len(src); i += 4 {
		binary.BigEndian.PutUint32(dst[i:], binary.LittleEndian.Uint32(src[i:]))
	}
}

func reorderF64Scalar(dst, src []byte) {
	for i := 0; i < len(src); i += 8 {
		binary.BigEndian.PutUint64(dst[i:], binary.LittleEndian.Uint64(src[i:]))
	}
}
