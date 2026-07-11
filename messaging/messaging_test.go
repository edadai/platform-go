package messaging

import "testing"

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}

	if cfg.NormalExchange() != DefaultExchange {
		t.Fatalf("NormalExchange() = %q", cfg.NormalExchange())
	}
	if cfg.DLX() != DefaultDeadLetterExchange {
		t.Fatalf("DLX() = %q", cfg.DLX())
	}
}
