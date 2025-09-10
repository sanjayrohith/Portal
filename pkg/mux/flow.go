package mux

import (
	"sync"

	"github.com/sanjayrohith/portal/pkg/protocol"
)

const (
	// DefaultInitialWindowSize is the default flow control credit per stream (256 KB).
	DefaultInitialWindowSize = 256 * 1024

	// DefaultReplenishThreshold triggers WINDOW_UPDATE when unreplenished consumed bytes exceed this amount.
	DefaultReplenishThreshold = DefaultInitialWindowSize / 2
)

// flowController manages sliding window credit for both outbound send and inbound receive directions.
type flowController struct {
	streamID uint32
	sender   frameSender

	// Outbound send window (credits granted by remote peer)
	sendMu     sync.Mutex
	sendCond   *sync.Cond
	sendWindow uint32

	// Inbound receive window (credits consumed locally that need replenishment)
	recvMu             sync.Mutex
	unreplenishedBytes uint32
	initialWindow      uint32
	threshold          uint32
}

// newFlowController initializes per-stream windowed flow control.
func newFlowController(streamID uint32, initialWindow uint32, sender frameSender) *flowController {
	if initialWindow == 0 {
		initialWindow = DefaultInitialWindowSize
	}
	fc := &flowController{
		streamID:      streamID,
		sender:        sender,
		sendWindow:    initialWindow,
		initialWindow: initialWindow,
		threshold:     initialWindow / 2,
	}
	fc.sendCond = sync.NewCond(&fc.sendMu)
	return fc
}

// acquireSendCredit acquires up to maxBytes of send credit. Blocks if sendWindow is zero.
func (fc *flowController) acquireSendCredit(maxBytes int, closeCh <-chan struct{}) (int, error) {
	fc.sendMu.Lock()
	defer fc.sendMu.Unlock()

	for fc.sendWindow == 0 {
		select {
		case <-closeCh:
			return 0, nil
		default:
		}
		fc.sendCond.Wait()
	}

	credit := int(fc.sendWindow)
	if credit > maxBytes {
		credit = maxBytes
	}

	fc.sendWindow -= uint32(credit)
	return credit, nil
}

// addSendCredit replenishes outbound send credit upon receiving a WINDOW_UPDATE frame.
func (fc *flowController) addSendCredit(delta uint32) {
	fc.sendMu.Lock()
	fc.sendWindow += delta
	fc.sendCond.Broadcast()
	fc.sendMu.Unlock()
}

// notifyConsumed records consumed payload bytes from Read() and triggers WINDOW_UPDATE if threshold is crossed.
func (fc *flowController) notifyConsumed(bytesRead int) error {
	if bytesRead <= 0 {
		return nil
	}

	fc.recvMu.Lock()
	fc.unreplenishedBytes += uint32(bytesRead)

	var creditToSend uint32
	if fc.unreplenishedBytes >= fc.threshold {
		creditToSend = fc.unreplenishedBytes
		fc.unreplenishedBytes = 0
	}
	fc.recvMu.Unlock()

	if creditToSend > 0 && fc.sender != nil {
		updateFrame := protocol.NewWindowUpdateFrame(fc.streamID, creditToSend)
		return fc.sender.sendFrame(updateFrame)
	}

	return nil
}

// AvailableSendCredit returns the current available outbound credit (useful for telemetry and tests).
func (fc *flowController) AvailableSendCredit() uint32 {
	fc.sendMu.Lock()
	defer fc.sendMu.Unlock()
	return fc.sendWindow
}
