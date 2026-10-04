package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"peringatan-dini/internal/config"
)

// captureNotifier mencatat pesan yang dikirim (untuk pengujian dispatch).
type captureNotifier struct {
	channel string
	sent    []Message
}

func (c *captureNotifier) Send(_ context.Context, msg Message) error {
	c.sent = append(c.sent, msg)
	return nil
}

func (c *captureNotifier) Channel() string { return c.channel }

func TestLogNotifier(t *testing.T) {
	n := NewLogNotifier("whatsapp")
	if n.Channel() != "whatsapp" {
		t.Errorf("channel = %q", n.Channel())
	}
	if err := n.Send(context.Background(), Message{To: "+62", Body: "hi"}); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestNewFromConfigLogMode(t *testing.T) {
	cfg := config.Config{NotifierMode: "log"}
	ns := NewFromConfig(cfg)
	if _, ok := ns["whatsapp"]; !ok {
		t.Fatal("whatsapp harus ada")
	}
	if _, ok := ns["sms"]; !ok {
		t.Fatal("sms harus ada")
	}
	if _, ok := ns["whatsapp"].(*LogNotifier); !ok {
		t.Fatal("mode log harus memakai LogNotifier")
	}
}

func TestNewFromConfigLiveMode(t *testing.T) {
	cfg := config.Config{NotifierMode: "live", WAEndpoint: "https://gw.example.com/send", WAKey: "k"}
	ns := NewFromConfig(cfg)
	if _, ok := ns["whatsapp"].(*HTTPNotifier); !ok {
		t.Fatal("mode live harus memakai HTTPNotifier")
	}
}

func TestHTTPNotifierWhatsAppPayload(t *testing.T) {
	var got map[string]any
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWhatsApp(srv.URL, "token-xyz")
	if err := n.Send(context.Background(), Message{To: "+62812", Body: "peringatan"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if authHeader != "Bearer token-xyz" {
		t.Errorf("auth header = %q", authHeader)
	}
	if got["to"] != "+62812" {
		t.Errorf("to = %v", got["to"])
	}
	if got["messaging_product"] != "whatsapp" {
		t.Errorf("messaging_product = %v", got["messaging_product"])
	}
	text, ok := got["text"].(map[string]any)
	if !ok || text["body"] != "peringatan" {
		t.Errorf("text payload salah: %v", got["text"])
	}
}

func TestHTTPNotifierSMSFrom(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSMS(srv.URL, "", "BMKG-JTG")
	if err := n.Send(context.Background(), Message{To: "62812", Body: "tes"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got["from"] != "BMKG-JTG" {
		t.Errorf("from = %v", got["from"])
	}
	if got["text"] != "tes" {
		t.Errorf("text = %v", got["text"])
	}
}

func TestHTTPNotifierErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewWhatsApp(srv.URL, "")
	if err := n.Send(context.Background(), Message{To: "+62", Body: "x"}); err == nil {
		t.Fatal("status 500 harus mengembalikan error")
	}
}
