package bytepool

import (
	math "math/rand/v2"
	"reflect"
	"sync/atomic"
	"unsafe"
)

const (
	PtrSize   = 4 << (^uintptr(0) >> 63)
	StrSize   = unsafe.Sizeof("")
	SliceSize = int(unsafe.Sizeof([]byte{}))

	MaxInt32   = 1<<31 - 1
	MaxUintptr = ^uintptr(0)
)

// usedElem represents an atomic boolean with padding to avoid false sharing.
type usedElem struct {
	val uint32
	_   [60]byte
}

// BytePool is a lock-free pool for managing byte slices efficiently.
type BytePool struct {
	shards []*bytePoolShard
	cap    uint
	len    uint
}

// bytePoolShard represents a shard containing a subset of byte slices.
type bytePoolShard struct {
	caches [][]byte
	used   []usedElem
	index  atomic.Uint64

	head unsafe.Pointer
	cap  uint
}

// NewBytePool initializes a new BytePool with sharding for better concurrency.
func NewBytePool(cap, max, shards uint) *BytePool {
	shardSize := max / shards

	pool := &BytePool{
		shards: make([]*bytePoolShard, shards),
		cap:    cap,
		len:    max,
	}
	var sh = int(shardSize * cap)
	for i := range pool.shards {
		data := MallocSlice[byte](sh, sh)
		caches := make([][]byte, shardSize)
		used := MallocSlice[usedElem](int(shardSize), int(shardSize))
		for j := uint(0); j < shardSize; j++ {
			caches[j] = data[j*cap : (j+1)*cap : (j+1)*cap][:0]
		}

		pool.shards[i] = &bytePoolShard{
			caches: caches,
			used:   used,
			head:   unsafe.Pointer(&data[0]),
			cap:    cap,
		}
	}
	return pool
}

type Iface struct {
	typ unsafe.Pointer
	ptr unsafe.Pointer
}

func Inspect(v interface{}) (reflect.Type, unsafe.Pointer) {

	return reflect.TypeOf(v), Pointer(v)
}

func Pointer(v interface{}) unsafe.Pointer {
	return (*Iface)(unsafe.Pointer(&v)).ptr
}

func MallocSlice[T any](len, cap int) []T {

	var t T
	mem, overflow := MulUintptr(unsafe.Sizeof(t), uintptr(cap))
	if overflow || len < 0 || len > cap {
		panic("invalid slice length or capacity")

	}
	return *(*[]T)(unsafe.Pointer(&struct {
		Data uintptr
		Len  int
		Cap  int
	}{uintptr(mallocgc(mem, Pointer(reflect.TypeOf(t)), false)), len, cap}))
}

func MulUintptr(a, b uintptr) (uintptr, bool) {
	if a|b < 1<<(4*PtrSize) || a == 0 {
		return a * b, false
	}
	overflow := b > MaxUintptr/a

	return a * b, overflow
}

//go:linkname mallocgc runtime.mallocgc
func mallocgc(size uintptr, typ unsafe.Pointer, needzero bool) unsafe.Pointer

func MakeNoZero(l uint) []byte {
	return unsafe.Slice((*byte)(mallocgc(uintptr(l), nil, false)), l) //  standart

}

// Get retrieves a byte slice from the pool, or allocates a new one if empty.
func (b *BytePool) Get() []byte {
	shard := b.shards[math.Uint32()%uint32(len(b.shards))]
	start := shard.index.Add(1) % uint64(len(shard.caches))
	for i := uint64(0); i < uint64(len(shard.caches)); i++ {
		idx := (start + i) % uint64(len(shard.caches))
		if atomic.CompareAndSwapUint32(&shard.used[idx].val, 0, 1) {
			return shard.caches[idx]
		}
	}
	return make([]byte, 0, b.cap)

}

// Put returns a byte slice back to the pool.
func (b *BytePool) Put(x []byte) {
	if cap(x) != int(b.cap) {
		return
	}
	for _, shard := range b.shards {

		ptr := unsafe.Pointer(&x[:1][0])
		if uintptr(ptr) >= uintptr(shard.head) && uintptr(ptr) < uintptr(shard.head)+uintptr(shard.cap*uint(len(shard.caches))) {
			i := (uintptr(ptr) - uintptr(shard.head)) / uintptr(shard.cap)
			atomic.StoreUint32(&shard.used[i].val, 0)
			return
		}
	}
}
