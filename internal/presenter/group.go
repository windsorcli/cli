// The Group is a set of named child operations that run side by side under one parent.
// It provides per-name start, completion, failure, and progress for waits over many resources.
// A child appears only once Start begins it, so resources still blocked on others stay hidden.
// Close ends every child still open, so no step is left running when the parent returns.

package presenter

import (
	"context"
	"sync"
)

// =============================================================================
// Types
// =============================================================================

// Group tracks one child operation per name under the operation in a context.
type Group struct {
	mu      sync.Mutex
	ctx     context.Context
	prefix  string
	order   []string
	ctxs    map[string]context.Context
	ends    map[string]func(error)
	pending map[string]string
	ended   map[string]bool
}

// =============================================================================
// Constructor
// =============================================================================

// NewGroup returns an empty group whose children start under the operation in ctx. Each child's
// subject is prefix followed by its name, and its message is the name.
func NewGroup(ctx context.Context, prefix string) *Group {
	return &Group{
		ctx:     ctx,
		prefix:  prefix,
		ctxs:    map[string]context.Context{},
		ends:    map[string]func(error){},
		pending: map[string]string{},
		ended:   map[string]bool{},
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Start begins the named child and shows any progress held for it. It does nothing for a name
// that has already started or ended.
func (g *Group) Start(name string) {
	g.mu.Lock()
	if _, open := g.ends[name]; open || g.ended[name] {
		g.mu.Unlock()
		return
	}
	ctx, end := Begin(g.ctx, g.prefix+name, name)
	g.ctxs[name], g.ends[name] = ctx, end
	g.order = append(g.order, name)
	text, held := g.pending[name]
	delete(g.pending, name)
	g.mu.Unlock()
	if held {
		Progress(ctx, text)
	}
}

// Done ends the named child as applied, starting it first when it has not started.
func (g *Group) Done(name string) {
	g.Start(name)
	g.end(name, nil)
}

// Fail ends the named child as failed with err, starting it first when it has not started.
func (g *Group) Fail(name string, err error) {
	g.Start(name)
	g.end(name, err)
}

// Progress replaces the status text of the named child. For a child that has not started, it
// holds the text until Start. It does nothing for a child that has ended.
func (g *Group) Progress(name, text string) {
	g.mu.Lock()
	ctx, open := g.ctxs[name]
	if !open && !g.ended[name] {
		g.pending[name] = text
	}
	g.mu.Unlock()
	if open {
		Progress(ctx, text)
	}
}

// Close ends every started child still open with err, in start order, so a nil err ends them as
// applied. Children that never started stay hidden.
func (g *Group) Close(err error) {
	g.mu.Lock()
	names := append([]string(nil), g.order...)
	g.mu.Unlock()
	for _, name := range names {
		g.end(name, err)
	}
}

// =============================================================================
// Private Methods
// =============================================================================

// end ends the named child with err and marks it ended.
func (g *Group) end(name string, err error) {
	g.mu.Lock()
	end, open := g.ends[name]
	delete(g.ends, name)
	delete(g.ctxs, name)
	if open {
		g.ended[name] = true
	}
	g.mu.Unlock()
	if open {
		end(err)
	}
}
