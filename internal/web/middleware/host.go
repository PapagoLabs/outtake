// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	fiber "github.com/gofiber/fiber/v3"
)

// hostPolicy decides which Host headers this installation answers.
type hostPolicy struct {
	// anyHost turns the check off.
	anyHost bool
	// exact holds the listed host names.
	exact map[string]struct{}
	// suffixes holds the listed domains, each with its leading dot.
	suffixes []string
}

const (
	// hostAllowAll is the allowlist entry that turns the host check off.
	hostAllowAll = "*"

	// labelSeparator separates the labels of a host name.
	labelSeparator = "."
)

// privateHostSuffixes are name suffixes no public DNS zone serves, so a page on
// another site cannot point one of them at this server.
var privateHostSuffixes = []string{
	".localhost",
	".local",
	".lan",
	".home",
	".home.arpa",
	".internal",
	".localdomain",
}

// HostGuard answers 421 Misdirected Request to a request whose Host header
// names a host this installation does not serve. A DNS rebinding page reaches
// a LAN server under its own host name, which this refuses.
//
// IP literals, localhost, single-label names, and names under suffixes no
// public DNS zone serves always pass, so LAN access needs no configuration.
// Any other name has to be listed.
//
// Parameters:
//   - allowed: Host names to answer, a leading-dot entry for a whole domain, or
//     "*" to answer every host.
//   - exempt: Request paths that skip the check, such as a health probe.
//
// Returns:
//   - handler: Middleware that refuses a request for an unlisted host.
func HostGuard(allowed []string, exempt ...string) fiber.Handler {
	policy := newHostPolicy(allowed)

	return func(ctx fiber.Ctx) error {
		host := ctx.Hostname()
		if policy.allows(host) || slices.Contains(exempt, ctx.Path()) {
			return ctx.Next()
		}

		log.Debug().Str("host", host).Msg("refused a request for an unlisted host")

		err := ctx.Status(fiber.StatusMisdirectedRequest).SendString(fmt.Sprintf(
			"Outtake does not answer to the host %q. Set OUTTAKE_PUBLIC_BASE_URL to the "+
				"address you open it at, or add the host to OUTTAKE_ALLOWED_HOSTS.",
			host,
		))
		if err != nil {
			return fmt.Errorf("write misdirected request: %w", err)
		}

		return nil
	}
}

// newHostPolicy reads an allowlist.
//
// Parameters:
//   - allowed: Host names, leading-dot domains, or "*".
//
// Returns:
//   - policy: The policy the guard applies.
func newHostPolicy(allowed []string) hostPolicy {
	policy := hostPolicy{
		anyHost:  false,
		exact:    make(map[string]struct{}, len(allowed)),
		suffixes: nil,
	}

	for _, entry := range allowed {
		entry = strings.TrimSuffix(
			strings.ToLower(strings.TrimSpace(entry)),
			labelSeparator,
		)

		switch {
		case entry == "":
		case entry == hostAllowAll:
			policy.anyHost = true
		case strings.HasPrefix(entry, labelSeparator):
			policy.suffixes = append(policy.suffixes, entry)
		default:
			policy.exact[entry] = struct{}{}
		}
	}

	return policy
}

// allows reports whether the policy answers a host.
//
// Parameters:
//   - host: Host header value without its port.
//
// Returns:
//   - allowed: True when the request may continue.
func (policy hostPolicy) allows(host string) bool {
	if policy.anyHost {
		return true
	}

	host = strings.TrimSuffix(strings.ToLower(host), labelSeparator)
	if host == "" {
		return false
	}

	return isIPLiteral(host) || isPrivateName(host) || policy.lists(host)
}

// lists reports whether the allowlist names a host or a domain above it.
//
// Parameters:
//   - host: Lowercase host name without a trailing dot.
//
// Returns:
//   - listed: True when an exact or domain entry matches.
func (policy hostPolicy) lists(host string) bool {
	if _, listed := policy.exact[host]; listed {
		return true
	}

	for _, suffix := range policy.suffixes {
		if strings.HasSuffix(host, suffix) || host == strings.TrimPrefix(suffix, labelSeparator) {
			return true
		}
	}

	return false
}

// isIPLiteral reports whether a host is an IPv4 or IPv6 address.
//
// Parameters:
//   - host: Host without its port. An IPv6 address may keep its brackets.
//
// Returns:
//   - literal: True for an IP address.
func isIPLiteral(host string) bool {
	_, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))

	return err == nil
}

// isPrivateName reports whether a host name cannot come from public DNS.
//
// Parameters:
//   - host: Lowercase host name.
//
// Returns:
//   - private: True for localhost, a single-label name, or a name under a
//     private-use suffix.
func isPrivateName(host string) bool {
	if host == "localhost" || !strings.Contains(host, labelSeparator) {
		return true
	}

	for _, suffix := range privateHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}

	return false
}
