package cedar

const (
	// ValueLimit limit value
	ValueLimit = int(^uint(0) >> 1)
)

/// Node contains the array of `base` and `check` as specified in the paper: "An efficient implementation of trie structures"
type node struct {
	Value int // if it is a negative value, then it stores the value of previous index that is free.
	Check int // if it is a negative value, then it stores the value of next index that is free.
}

func (n *node) base() int {
	return -(n.Value + 1)
}

type nvalue struct {
	Len   int
	Value interface{}
}

/// NInfo stores the information about the trie
type ninfo struct {
	Sibling byte // the index of right sibling, it is 0 if it doesn't have a sibling.
	Child   byte // the index of the first child
	End     bool
}

/// Block stores the linked-list pointers and the stats info for blocks.
type block struct {
	Prev   int // previous block's index
	Next   int // next block's index
	Num    int // the number of slots that is free, the range is 0-256
	Reject int // a heuristic number to make the search for free space faster,
	// it is the minimum number of iteration in each trie node it has to try before we can conclude that we can reject this block.
	// If the number of kids for the block we are looking for is less than this number then this block is worthy of searching.
	Trial int // the number of times this block has been probed by `find_places` for the free block.
	Ehead int // the index of the first empty element in this block
}

func (b *block) init() {
	b.Num = 256
	b.Reject = 257
}

// Cedar cedar struct
/// `Cedar` holds all of the information about double array trie.
type Cedar struct {
	Array  []node // storing the `base` and `check` info from the original paper.
	Ninfos []ninfo
	Blocks []block
	Reject [257]int

	vals map[int]nvalue
	vkey int

	BheadF int // the index of the first 'Full' block, 0 means no 'Full' block
	BheadC int // the index of the first 'Closed' block, 0 means no ' Closed' block
	BheadO int // the index of the first 'Open' block, 0 means no 'Open' block

	Capacity int
	Size     int
	Ordered  bool
	MaxTrial int // the parameter for cedar, it could be tuned for more, but the default is 1.
}

// New new Cedar
func New() *Cedar {
	da := Cedar{
		Array:    make([]node, 256),
		Ninfos:   make([]ninfo, 256),
		Blocks:   make([]block, 1),
		vals:     make(map[int]nvalue),
		vkey:     1,
		Capacity: 256,
		Size:     256,
		Ordered:  true,
		MaxTrial: 1,
	}

	da.Array[0] = node{-2, 0}
	for i := 1; i < 256; i++ {
		// make `base` point to the previous element, and make `check` point to the next element
		da.Array[i] = node{-(i - 1), -(i + 1)}
	}
	// make them link as a cyclic doubly-linked list
	da.Array[1].Value = -255
	da.Array[255].Check = -1

	da.Blocks[0].Ehead = 1
	da.Blocks[0].init()

	for i := 0; i <= 256; i++ {
		da.Reject[i] = i + 1
	}

	return &da
}

// Get value by key, insert the key if not exist
func (da *Cedar) get(key []byte, from, pos int) *int {
	to := da.getV(key, from, pos)
	return &da.Array[to].Value
}

// GetV value by key, insert the key if not exist
func (da *Cedar) getV(key []byte, from, pos int) int {
	for ; pos < len(key); pos++ {
		if value := da.Array[from].Value; value >= 0 && value != ValueLimit {
			to := da.follow(from, 0)
			da.Array[to].Value = value
		}
		// key
		from = da.follow(from, key[pos])
	}

	to := from
	if da.Array[from].Value < 0 {
		to = da.follow(from, 0)
	}

	return to
}

func (da *Cedar) vKey() int {
	k := da.vkey
	for {
		k = (k + 1) % da.Capacity
		if _, ok := da.vals[k]; !ok {
			break
		}
	}

	da.vkey = k
	return k
}

func (da *Cedar) follow(from int, label byte) int {
	base := da.Array[from].base()
	to := base ^ int(label) // 对应状态转移关系： 「base[s]+c=t」，to 代表转移到的状态

	// da.Array[to].Check < 0 : to 位置为空；
	// base < 0：有 tail 数组？
	if base < 0 || da.Array[to].Check < 0 {
		hasChild := false
		if base >= 0 {
			i := base ^ int(da.Ninfos[from].Child)
			// da.Array[i].Check == from 表示位置 i 处的状态由 from 处转移而来
			hasChild = da.Array[i].Check == from
		}
		to = da.popEnode(base, from, label)
		da.pushSibling(from, to^int(label), label, hasChild)

		return to
	}

	// Check 值不为负数，且父状态不是 from，则需要解决冲突
	if da.Array[to].Check != from {
		to = da.resolve(from, base, label)
		return to
	}

	if da.Array[to].Check == from {
		return to
	}

	panic("Cedar: internal error, should not be here")
	// return to
}

func (da *Cedar) popBlock(bi int, headIn *int, last bool) {
	if last {
		*headIn = 0
		return
	}

	b := &da.Blocks[bi]
	da.Blocks[b.Prev].Next = b.Next
	da.Blocks[b.Next].Prev = b.Prev
	if bi == *headIn {
		*headIn = b.Next
	}
}

func (da *Cedar) pushBlock(bi int, headOut *int, empty bool) {
	b := &da.Blocks[bi]
	if empty {
		*headOut, b.Prev, b.Next = bi, bi, bi
	} else {
		tailOut := &da.Blocks[*headOut].Prev
		b.Prev = *tailOut
		b.Next = *headOut
		*headOut, *tailOut, da.Blocks[*tailOut].Next = bi, bi, bi
	}
}

func (da *Cedar) addBlock() int {
	if da.Size == da.Capacity {
		da.Capacity *= 2

		oldArray := da.Array
		da.Array = make([]node, da.Capacity)
		copy(da.Array, oldArray)

		oldNinfo := da.Ninfos
		da.Ninfos = make([]ninfo, da.Capacity)
		copy(da.Ninfos, oldNinfo)

		oldBlock := da.Blocks
		da.Blocks = make([]block, da.Capacity>>8)
		copy(da.Blocks, oldBlock)
	}

	da.Blocks[da.Size>>8].init()
	da.Blocks[da.Size>>8].Ehead = da.Size

	da.Array[da.Size] = node{-(da.Size + 255), -(da.Size + 1)}
	for i := da.Size + 1; i < da.Size+255; i++ {
		da.Array[i] = node{-(i - 1), -(i + 1)}
	}
	da.Array[da.Size+255] = node{-(da.Size + 254), -da.Size}

	da.pushBlock(da.Size>>8, &da.BheadO, da.BheadO == 0)
	da.Size += 256
	return da.Size>>8 - 1
}

func (da *Cedar) transferBlock(bi int, headIn, headOut *int) {
	da.popBlock(bi, headIn, bi == da.Blocks[bi].Next)
	da.pushBlock(bi, headOut, *headOut == 0 && da.Blocks[bi].Num != 0)
}

func (da *Cedar) popEnode(base, from int, label byte) int {
	e := base ^ int(label) // to：目的状态
	if base < 0 {
		e = da.findPlace()
	}

	// block index
	bi := e >> 8
	n := &da.Array[e]
	b := &da.Blocks[bi]
	b.Num--

	if b.Num == 0 {
		// block 空间耗尽？？
		if bi != 0 {
			da.transferBlock(bi, &da.BheadC, &da.BheadF)
		}
	} else {
		da.Array[-n.Value].Check = n.Check
		da.Array[-n.Check].Value = n.Value

		if e == b.Ehead {
			b.Ehead = -n.Check
		}

		if bi != 0 && b.Num == 1 && b.Trial != da.MaxTrial {
			da.transferBlock(bi, &da.BheadO, &da.BheadC)
		}
	}

	n.Value = ValueLimit
	// 设置父子状态
	n.Check = from
	if base < 0 {
		// 等价于前面计算 base 的方法：n.base() = -(da.Array[from].Value + 1) = e ^ int(label))
		da.Array[from].Value = -(e ^ int(label)) - 1
	}

	return e
}

func (da *Cedar) pushEnode(e int) {
	bi := e >> 8
	b := &da.Blocks[bi]
	b.Num++

	if b.Num == 1 {
		b.Ehead = e
		da.Array[e] = node{-e, -e}
		if bi != 0 {
			da.transferBlock(bi, &da.BheadF, &da.BheadC)
		}
	} else {
		prev := b.Ehead
		next := -da.Array[prev].Check
		da.Array[e] = node{-prev, -next}
		da.Array[prev].Check = -e
		da.Array[next].Value = -e

		if b.Num == 2 || b.Trial == da.MaxTrial {
			if bi != 0 {
				da.transferBlock(bi, &da.BheadC, &da.BheadO)
			}
		}

		b.Trial = 0
	}

	if b.Reject < da.Reject[b.Num] {
		b.Reject = da.Reject[b.Num]
	}
	da.Ninfos[e] = ninfo{}
}

// hasChild: whether the `from` node has children
func (da *Cedar) pushSibling(from, base int, label byte, hasChild bool) {
	c := &da.Ninfos[from].Child
	keepOrder := *c == 0
	if da.Ordered {
		keepOrder = label > *c
	}

	if hasChild && keepOrder {
		c = &da.Ninfos[base^int(*c)].Sibling
		for da.Ordered && *c != 0 && *c < label {
			c = &da.Ninfos[base^int(*c)].Sibling
		}
	}

	// base^int(label) 貌似是一个转移规则，多次出现
	da.Ninfos[base^int(label)].Sibling = *c
	*c = label
}

func (da *Cedar) popSibling(from, base int, label byte) {
	c := &da.Ninfos[from].Child
	for *c != label {
		c = &da.Ninfos[base^int(*c)].Sibling
	}
	*c = da.Ninfos[base^int(*c)].Sibling
}

func (da *Cedar) consult(baseN, baseP int, cN, cP byte) bool {
	cN = da.Ninfos[baseN^int(cN)].Sibling
	cP = da.Ninfos[baseP^int(cP)].Sibling

	for cN != 0 && cP != 0 {
		cN = da.Ninfos[baseN^int(cN)].Sibling
		cP = da.Ninfos[baseP^int(cP)].Sibling
	}
	return cP != 0
}

func (da *Cedar) setChild(base int, c, label byte, flag bool) []byte {
	child := make([]byte, 0, 257)
	if c == 0 {
		child = append(child, c)
		c = da.Ninfos[base^int(c)].Sibling
	}

	if da.Ordered {
		for c != 0 && c <= label {
			child = append(child, c)
			c = da.Ninfos[base^int(c)].Sibling
		}
	}

	if flag {
		child = append(child, label)
	}

	for c != 0 {
		child = append(child, c)
		c = da.Ninfos[base^int(c)].Sibling
	}

	return child
}

func (da *Cedar) findPlace() int {
	if da.BheadC != 0 {
		return da.Blocks[da.BheadC].Ehead
	}

	if da.BheadO != 0 {
		return da.Blocks[da.BheadO].Ehead
	}

	// 扩容？？？
	return da.addBlock() << 8
}

func (da *Cedar) findPlaces(child []byte) int {
	bi := da.BheadO
	if bi != 0 {
		e := da.listBi(bi, child)
		if e > 0 {
			return e
		}
	}

	return da.addBlock() << 8
}

func (da *Cedar) listBi(bi int, child []byte) int {
	nc := len(child)
	bz := da.Blocks[da.BheadO].Prev

	for {
		b := &da.Blocks[bi]
		if b.Num >= nc && nc < b.Reject {
			e := da.listEhead(b, child)
			if e > 0 {
				return e
			}
		}

		b.Reject = nc
		if b.Reject < da.Reject[b.Num] {
			da.Reject[b.Num] = b.Reject
		}

		biN := b.Next
		b.Trial++
		if b.Trial == da.MaxTrial {
			da.transferBlock(bi, &da.BheadO, &da.BheadC)
		}

		if bi == bz {
			break
		}
		bi = biN
	}

	return 0
}

func (da *Cedar) listEhead(b *block, child []byte) int {
	for e := b.Ehead; ; {
		base := e ^ int(child[0])
		for i := 0; da.Array[base^int(child[i])].Check < 0; i++ {
			if i == len(child)-1 {
				b.Ehead = e
				// if e == 0 {
				// }
				return e
			}
		}

		e = -da.Array[e].Check
		if e == b.Ehead {
			break
		}
	}

	return 0
}

func (da *Cedar) resolve(fromN, baseN int, labelN byte) int {
	toPn := baseN ^ int(labelN)
	fromP := da.Array[toPn].Check
	baseP := da.Array[fromP].base()
	flag := da.consult(baseN, baseP, da.Ninfos[fromN].Child, da.Ninfos[fromP].Child)

	var children []byte
	if flag {
		children = da.setChild(baseN, da.Ninfos[fromN].Child, labelN, true)
	} else {
		children = da.setChild(baseP, da.Ninfos[fromP].Child, 255, false)
	}

	var base int
	if len(children) == 1 {
		base = da.findPlace()
	} else {
		base = da.findPlaces(children)
	}
	base ^= int(children[0])

	var (
		from  int
		nbase int
	)

	if flag {
		from = fromN
		nbase = baseN
	} else {
		from = fromP
		nbase = baseP
	}

	if flag && children[0] == labelN {
		da.Ninfos[from].Child = labelN
	}

	da.Array[from].Value = -base - 1
	base, labelN, toPn = da.list(base, from, nbase, fromN, toPn,
		labelN, children, flag)

	if flag {
		return base ^ int(labelN)
	}

	return toPn
}

func (da *Cedar) list(base, from, nbase, fromN, toPn int,
	labelN byte, children []byte, flag bool) (int, byte, int) {
	for i := 0; i < len(children); i++ {
		to := da.popEnode(base, from, children[i])
		newTo := nbase ^ int(children[i])

		if i == len(children)-1 {
			da.Ninfos[to].Sibling = 0
		} else {
			da.Ninfos[to].Sibling = children[i+1]
		}

		if flag && newTo == toPn { // new node has no child
			continue
		}

		n := &da.Array[to]
		ns := &da.Array[newTo]
		n.Value = ns.Value

		if n.Value < 0 && children[i] != 0 {
			// this node has children, fix their check
			c := da.Ninfos[newTo].Child
			da.Ninfos[to].Child = c
			da.Array[n.base()^int(c)].Check = to

			c = da.Ninfos[n.base()^int(c)].Sibling
			for c != 0 {
				da.Array[n.base()^int(c)].Check = to
				c = da.Ninfos[n.base()^int(c)].Sibling
			}
		}

		if !flag && newTo == fromN { // parent node moved
			fromN = to
		}

		if !flag && newTo == toPn {
			da.pushSibling(fromN, toPn^int(labelN), labelN, true)
			da.Ninfos[newTo].Child = 0
			ns.Value = ValueLimit
			ns.Check = fromN
		} else {
			da.pushEnode(newTo)
		}
	}

	return base, labelN, toPn
}
