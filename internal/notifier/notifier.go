// Package notifier menyediakan abstraksi pengiriman pesan (WhatsApp & SMS).
//
// Desain: interface Notifier + beberapa implementasi. LogNotifier dipakai untuk
// belajar (gratis), sedangkan WhatsAppNotifier & SMSNotifier untuk produksi.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"peringatan-dini/internal/config"
)

// Message adalah pesan yang akan dikirim.
type Message struct {
	To   string
	Body string
}

// Notifier mengirim pesan ke nomor tujuan.
type Notifier interface {
	Send(ctx context.Context, msg Message) error
	Channel() string
}

// LogNotifier menuliskan pesan ke log (mode belajar, gratis).
type LogNotifier struct {
	channel string
}

// NewLogNotifier membuat notifier yang hanya mencatat ke log.
func NewLogNotifier(channel string) *LogNotifier {
	return &LogNotifier{channel: channel}
}

// Send mencatat pesan ke log.
func (l *LogNotifier) Send(_ context.Context, msg Message) error {
	slog.Info("notifikasi (mode log)",
		"channel", l.channel,
		"to_masked", msg.To,
		"body", msg.Body,
	)
	return nil
}

// Channel mengembalikan nama kanal.
func (l *LogNotifier) Channel() string { return l.channel }

// HTTPNotifier mengirim pesan lewat HTTP generik (untuk gateway WA/SMS).
// Payload disesuaikan lewat field template.
type HTTPNotifier struct {
	channel  string
	endpoint string
	apiKey   string
	from     string
	client   *http.Client
	buildReq func(from, to, body string) any
}

// NewWhatsApp membuat notifier WhatsApp (via gateway / WhatsApp Cloud API).
func NewWhatsApp(endpoint, apiKey string) *HTTPNotifier {
	return &HTTPNotifier{
		channel:  "whatsapp",
		endpoint: endpoint,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 15 * time.Second},
		buildReq: func(_, to, body string) any {
			return map[string]any{
				"messaging_product": "whatsapp",
				"to":                to,
				"type":              "text",
				"text":              map[string]string{"body": body},
			}
		},
	}
}

// NewSMS membuat notifier SMS (via provider lokal).
func NewSMS(endpoint, apiKey, from string) *HTTPNotifier {
	return &HTTPNotifier{
		channel:  "sms",
		endpoint: endpoint,
		apiKey:   apiKey,
		from:     from,
		client:   &http.Client{Timeout: 15 * time.Second},
		buildReq: func(from, to, body string) any {
			return map[string]any{
				"from": from,
				"to":   to,
				"text": body,
			}
		},
	}
}

// Send mengirim pesan via HTTP POST JSON.
func (h *HTTPNotifier) Send(ctx context.Context, msg Message) error {
	payload := h.buildReq(h.from, msg.To, msg.Body)
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway %s mengembalikan status %d", h.channel, resp.StatusCode)
	}
	return nil
}

// Channel mengembalikan nama kanal.
func (h *HTTPNotifier) Channel() string { return h.channel }

// NewFromConfig membuat notifier sesuai konfigurasi.
//
//	switch NOTIFIER_MODE=log  -> LogNotifier (gratis, untuk belajar)
//	switch NOTIFIER_MODE=live -> WhatsApp & SMS via gateway
func NewFromConfig(cfg config.Config) map[string]Notifier {
	if cfg.NotifierMode == "live" && cfg.WAEndpoint != "" {
		return map[string]Notifier{
			"whatsapp": NewWhatsApp(cfg.WAEndpoint, cfg.WAKey),
			"sms":      NewSMS(cfg.SMSProvider, cfg.SMSKey, cfg.SMSFrom),
		}
	}
	return map[string]Notifier{
		"whatsapp": NewLogNotifier("whatsapp"),
		"sms":      NewLogNotifier("sms"),
	}
}
