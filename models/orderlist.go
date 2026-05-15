package models

// OrderNode is one entry in a level's FIFO queue. It carries the Order
// itself plus prev/next pointers for O(1) removal, and a back-reference
// to the level so a cancel-by-orderID lookup can locate the price level
// without scanning the ladder.
//
// OrderNode is not safe for concurrent use; the matching engine guarantees
// single-writer access to each Orderbook (one goroutine per symbol).
type OrderNode struct {
	Order Order
	Prev  *OrderNode
	Next  *OrderNode
	Level *MatchEngineEntry
}

// OrderList is a doubly-linked list of orders maintained in arrival order
// (head = oldest, tail = newest). This is the storage at each price level.
//
// All operations are O(1) given a node reference. The matching loop only
// ever removes from the head; cancel removes at an arbitrary position via
// the orderID index on Orderbook.
type OrderList struct {
	head *OrderNode
	tail *OrderNode
	size int
}

// Head returns the oldest order's node, or nil if the list is empty.
// Matching consumes from the head.
func (l *OrderList) Head() *OrderNode { return l.head }

// Len reports how many orders are in the list.
func (l *OrderList) Len() int { return l.size }

// IsEmpty reports whether the list has no orders.
func (l *OrderList) IsEmpty() bool { return l.size == 0 }

// PushBack appends an order to the tail (newest position) and returns
// the node so the caller can register it in an orderID index.
func (l *OrderList) PushBack(o Order, level *MatchEngineEntry) *OrderNode {
	n := &OrderNode{Order: o, Level: level}
	if l.tail == nil {
		l.head = n
	} else {
		n.Prev = l.tail
		l.tail.Next = n
	}
	l.tail = n
	l.size++
	return n
}

// Remove unlinks n from the list. n must already be a member of l;
// double-removal will corrupt the list.
func (l *OrderList) Remove(n *OrderNode) {
	if n.Prev != nil {
		n.Prev.Next = n.Next
	} else {
		l.head = n.Next
	}
	if n.Next != nil {
		n.Next.Prev = n.Prev
	} else {
		l.tail = n.Prev
	}
	n.Prev, n.Next = nil, nil
	l.size--
}
