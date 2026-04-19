// Package flux provides Flux kustomization stack management functionality.
// This file contains the production port-forward implementation the Notifier
// uses to reach the in-cluster webhook-receiver Service without depending on
// the kubectl CLI. It relies on client-go's spdy transport to open a
// port-forward directly against the Pod's portforward subresource.

package flux

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// =============================================================================
// Types
// =============================================================================

// nopWriter discards all writes so client-go's port-forward info lines
// (for example "Forwarding from 127.0.0.1:xxxxx -> 80") never leak into
// the user's CLI output for a background notification call.
type nopWriter struct{}

// Write implements io.Writer by discarding all input.
func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// =============================================================================
// Helpers
// =============================================================================

// productionPortForward opens a real port-forward through the Kubernetes API
// server to the named Pod and returns the local loopback URL the caller can
// POST to. The returned stop function both tears down the forward and blocks
// until the forwarder goroutine has exited, ensuring callers never return
// with stray goroutines still running past their notifyTimeout budget; stop
// returns the forwarder's exit error when it terminated after readyCh fired
// (the only window where such an error is otherwise unobservable). An empty
// local port (0:<remote>) lets the OS pick an unused ephemeral port which is
// then read back via GetPorts after readyCh fires. A secondary goroutine
// translates ctx cancellation into a stop signal so a parent context that
// fires during the POST phase still tears the forward down deterministically.
func productionPortForward(ctx context.Context, cfg *rest.Config, namespace, podName string, remotePort int) (string, func() error, error) {
	if cfg == nil {
		return "", nil, fmt.Errorf("rest config is nil")
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("build typed clientset: %w", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("build spdy transport: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	ports := []string{fmt.Sprintf("0:%d", remotePort)}

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, nopWriter{}, nopWriter{})
	if err != nil {
		return "", nil, fmt.Errorf("build port-forwarder: %w", err)
	}

	// done is closed after the forwarder goroutine exits, letting stop() wait
	// for full teardown and letting the select below observe an early exit.
	// fwErr is written only before done is closed, so the synchronization with
	// <-done makes the subsequent read data-race free.
	done := make(chan struct{})
	var fwErr error
	go func() {
		defer close(done)
		if err := fw.ForwardPorts(); err != nil {
			fwErr = err
		}
	}()

	stopOnce := func() {
		select {
		case stopCh <- struct{}{}:
		default:
		}
	}

	// linkDone closes after the ctx->stop linker exits, so stop() can wait
	// for that helper too and leave no stray goroutines behind.
	linkDone := make(chan struct{})
	go func() {
		defer close(linkDone)
		select {
		case <-ctx.Done():
			stopOnce()
		case <-done:
		}
	}()

	stop := func() error {
		stopOnce()
		<-done
		<-linkDone
		return fwErr
	}

	select {
	case <-readyCh:
	case <-done:
		_ = stop()
		if fwErr != nil {
			return "", nil, fmt.Errorf("forward ports: %w", fwErr)
		}
		return "", nil, fmt.Errorf("forward ports exited before ready")
	case <-ctx.Done():
		_ = stop()
		return "", nil, ctx.Err()
	}

	assigned, err := fw.GetPorts()
	if err != nil {
		_ = stop()
		return "", nil, fmt.Errorf("get forwarded ports: %w", err)
	}
	if len(assigned) == 0 {
		_ = stop()
		return "", nil, fmt.Errorf("no ports forwarded")
	}

	return fmt.Sprintf("http://127.0.0.1:%d", assigned[0].Local), stop, nil
}

// =============================================================================
// Interface Compliance
// =============================================================================

// Ensure productionPortForward matches the NotifierShims.PortForward signature.
var _ func(context.Context, *rest.Config, string, string, int) (string, func() error, error) = productionPortForward
