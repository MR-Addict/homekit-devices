package wol

import (
	"context"
	"net"
	"reflect"
	"testing"
)

func TestBuildMagicPacket(t *testing.T) {
	hardwareAddr, err := net.ParseMAC("00:11:22:33:44:55")
	if err != nil {
		t.Fatalf("ParseMAC() returned error: %v", err)
	}

	packet, err := BuildMagicPacket(hardwareAddr)
	if err != nil {
		t.Fatalf("BuildMagicPacket() returned error: %v", err)
	}

	if len(packet) != 102 {
		t.Fatalf("expected 102-byte packet, got %d", len(packet))
	}

	for index := 0; index < 6; index++ {
		if packet[index] != 0xFF {
			t.Fatalf("expected packet byte %d to be 0xFF, got %#x", index, packet[index])
		}
	}

	for offset := 6; offset < len(packet); offset += len(hardwareAddr) {
		segment := packet[offset : offset+len(hardwareAddr)]
		if string(segment) != string(hardwareAddr) {
			t.Fatalf("unexpected MAC payload at offset %d: %v", offset, segment)
		}
	}
}

func TestBuildMagicPacketRejectsShortMAC(t *testing.T) {
	_, err := BuildMagicPacket(net.HardwareAddr{0x01, 0x02})
	if err == nil {
		t.Fatal("expected invalid MAC length to fail")
	}
}

func TestSendRejectsInvalidMAC(t *testing.T) {
	err := Send(context.Background(), "not-a-mac", "255.255.255.255", 9)
	if err == nil {
		t.Fatal("expected invalid MAC to fail")
	}
}

func TestResolveBroadcastTargetsKeepsDirectedBroadcast(t *testing.T) {
	targets := resolveBroadcastTargets("192.168.1.255", nil)
	want := []string{"192.168.1.255"}

	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("resolveBroadcastTargets() = %v, want %v", targets, want)
	}
}

func TestResolveBroadcastTargetsExpandsLimitedBroadcast(t *testing.T) {
	targets := resolveBroadcastTargets("255.255.255.255", []string{"192.168.1.255", "10.0.0.255", "192.168.1.255", "not-an-ip"})
	want := []string{"255.255.255.255", "192.168.1.255", "10.0.0.255"}

	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("resolveBroadcastTargets() = %v, want %v", targets, want)
	}
}
