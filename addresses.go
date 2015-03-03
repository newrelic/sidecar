package main

import (
	"errors"
	"net"
)

var privateBlocks []*net.IPNet

func setupIPBlocks() {
	privateBlockStrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	privateBlocks = make([]*net.IPNet, len(privateBlockStrs))

	for i, blockStr := range(privateBlockStrs) {
		_, block, _ := net.ParseCIDR(blockStr)
		privateBlocks[i] = block
	}
}

func isPrivateIP(ip_str string) bool {
	ip := net.ParseIP(ip_str)

	for _, priv := range privateBlocks {
		if priv.Contains(ip) {
			return true
		}
	}
	return false
}

func findPrivateAddresses() ([]*net.IP, error) {
	if len(privateBlocks) < 1 {
		setupIPBlocks()
	}

	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, errors.New(
			"Failed to get interface addresses! Err: " + err.Error(),
		)
	}

	result := make([]*net.IP, 0, len(addresses))

	// Find private IPv4 address
	for _, rawAddr := range addresses {
		var ip net.IP
		switch addr := rawAddr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}

		if ip.To4() == nil {
			continue
		}

		if(isPrivateIP(ip.String())) {
			result = append(result, &ip)
		}
	}

	err = nil

	if len(result) < 1 {
		err = errors.New("No addresses found!")
		result = nil
	}

	return result, err
}

func getPublishedIP(excluded []string) (string, error) {
	addresses, _ := findPrivateAddresses()

	skip := false
	for _, address := range addresses {
		for _, excludeIP := range excluded {
			if address.String() == excludeIP {
				skip = true
				break
			}
		}
		if skip {
			skip = false
			continue
		}
		return address.String(), nil
	}

	return "", errors.New("Can't find address!")
}
