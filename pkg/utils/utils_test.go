package utils

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOutboundIP(t *testing.T) {
	ip := GetOutboundIP()
	assert.NotNil(t, ip)
	assert.True(t, ip.IsGlobalUnicast() || ip.IsPrivate(), "IP should be global unicast or private address")
}

func TestMaddrToHostPort(t *testing.T) {
	tests := []struct {
		name     string
		maddr    string
		wantIP   string
		wantPort int32
		wantErr  bool
	}{
		{
			name:     "valid IPv4 address",
			maddr:    "/ip4/198.18.0.1/tcp/43480",
			wantIP:   "198.18.0.1",
			wantPort: 43480,
			wantErr:  false,
		},
		{
			name:     "invalid address format",
			maddr:    "invalid-addr",
			wantIP:   "",
			wantPort: 0,
			wantErr:  true,
		},
		{
			name:     "port number out of range",
			maddr:    "/ip4/198.18.0.1/tcp/70000",
			wantIP:   "",
			wantPort: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, port, err := MaddrToHostPort(tt.maddr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantIP, ip)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestNormalizeImage(t *testing.T) {
	tests := []struct {
		name      string
		imageAddr string
		wantTag   string
		wantErr   bool
	}{
		{
			name:      "complete image address",
			imageAddr: "nginx:1.19",
			wantTag:   "1.19",
			wantErr:   false,
		},
		{
			name:      "image address without tag",
			imageAddr: "nginx",
			wantTag:   "latest",
			wantErr:   false,
		},
		{
			name:      "invalid image address",
			imageAddr: "invalid::::image",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := normalizeImage(tt.imageAddr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantTag, ref.Tag())
		})
	}
}

func TestMapToString(t *testing.T) {
	m := &sync.Map{}
	m.Store("key1", "value1")
	m.Store("key2", 2)

	result := MapToString(m)
	assert.Contains(t, result, "key1: value1")
	assert.Contains(t, result, "key2: 2")
}
