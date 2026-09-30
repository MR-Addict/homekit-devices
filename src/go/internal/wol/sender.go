package wol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const limitedBroadcastIP = "255.255.255.255"

func BuildMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("mac address must be 6 bytes")
	}

	packet := make([]byte, 6+(16*len(mac)))
	for index := range 6 {
		packet[index] = 0xFF
	}
	for index := 6; index < len(packet); index += len(mac) {
		copy(packet[index:index+len(mac)], mac)
	}

	return packet, nil
}

func Send(ctx context.Context, mac, broadcastIP string, port int) error {
	hardwareAddr, err := net.ParseMAC(strings.TrimSpace(mac))
	if err != nil {
		return fmt.Errorf("parse mac address: %w", err)
	}

	packet, err := BuildMagicPacket(hardwareAddr)
	if err != nil {
		return err
	}

	targets := resolveBroadcastTargets(broadcastIP, localBroadcastTargets())
	if len(targets) == 0 {
		return fmt.Errorf("resolve broadcast targets: no valid IPv4 broadcast targets")
	}

	var sendErrors []error
	for _, target := range targets {
		if err := sendPacket(ctx, packet, target, port); err != nil {
			sendErrors = append(sendErrors, err)
			continue
		}

		return nil
	}

	return errors.Join(sendErrors...)
}

func sendPacket(ctx context.Context, packet []byte, broadcastIP string, port int) error {

	address := net.JoinHostPort(strings.TrimSpace(broadcastIP), strconv.Itoa(port))
	dialer := net.Dialer{
		Control: enableBroadcastSocket,
	}

	conn, err := dialer.DialContext(ctx, "udp4", address)
	if err != nil {
		return fmt.Errorf("dial broadcast address: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
	}

	written, err := conn.Write(packet)
	if err != nil {
		return fmt.Errorf("send magic packet: %w", err)
	}
	if written != len(packet) {
		return fmt.Errorf("send magic packet: wrote %d of %d bytes", written, len(packet))
	}

	return nil
}

func resolveBroadcastTargets(configured string, interfaceBroadcasts []string) []string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return nil
	}

	seen := map[string]struct{}{}
	targets := make([]string, 0, 1+len(interfaceBroadcasts))
	appendTarget := func(ip string) {
		if !isValidIPv4(ip) {
			return
		}
		if _, exists := seen[ip]; exists {
			return
		}
		seen[ip] = struct{}{}
		targets = append(targets, ip)
	}

	appendTarget(configured)
	if configured != limitedBroadcastIP {
		return targets
	}

	for _, ip := range interfaceBroadcasts {
		appendTarget(ip)
	}

	return targets
}

func localBroadcastTargets() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	targets := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if !ok {
				continue
			}

			if broadcast := directedBroadcast(ipNet); broadcast != "" {
				targets = append(targets, broadcast)
			}
		}
	}

	return targets
}

func directedBroadcast(ipNet *net.IPNet) string {
	if ipNet == nil || ipNet.IP == nil || ipNet.Mask == nil {
		return ""
	}

	ip := ipNet.IP.To4()
	mask := ipNet.Mask
	if ip == nil || len(mask) != net.IPv4len {
		return ""
	}

	broadcast := make(net.IP, net.IPv4len)
	for index := range net.IPv4len {
		broadcast[index] = ip[index] | ^mask[index]
	}

	return broadcast.String()
}

func isValidIPv4(address string) bool {
	ip := net.ParseIP(strings.TrimSpace(address))
	return ip != nil && ip.To4() != nil
}
