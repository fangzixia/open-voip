package ws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"

	"open-call/internal/datetime"
	"open-call/internal/layers/biz/auth"
)

const clientOutboundCap = 256

// errSlowConsumer 表示客户端发送队列已满；连接会被关闭，客户端按 since 重连补发。
var errSlowConsumer = errors.New("ws: slow consumer")

type client struct {
	conn      *websocket.Conn
	principal auth.Principal
	outbound  chan envelope
	done      chan struct{}
	once      sync.Once
}

func newClient(conn *websocket.Conn, p auth.Principal) *client {
	cl := &client{
		conn:      conn,
		principal: p,
		outbound:  make(chan envelope, clientOutboundCap),
		done:      make(chan struct{}),
	}
	go cl.writeLoop()
	return cl
}

// close 只关闭 done，不关闭 outbound，避免与并发 send 竞争导致 panic。
func (c *client) close() {
	c.once.Do(func() { close(c.done) })
}

func (c *client) kick(code websocket.StatusCode, reason string) {
	c.close()
	_ = c.conn.Close(code, reason)
}

func (c *client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.outbound:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.writeDirect(ctx, msg)
			cancel()
			if err != nil {
				c.kick(websocket.StatusInternalError, "write failed")
				return
			}
		}
	}
}

func (c *client) writeDirect(ctx context.Context, v any) error {
	b, err := datetime.Marshal(v)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, b)
}

// sendWait 阻塞等待队列空位，用于补发等批量写入场景。
func (c *client) sendWait(ctx context.Context, msg envelope) error {
	select {
	case <-c.done:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	case c.outbound <- msg:
		return nil
	}
}

func (c *client) send(ctx context.Context, v any) error {
	msg, ok := v.(envelope)
	if !ok {
		return c.writeDirect(ctx, v)
	}
	select {
	case <-c.done:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	case c.outbound <- msg:
		return nil
	default:
		c.kick(websocket.StatusTryAgainLater, "slow consumer")
		return errSlowConsumer
	}
}
