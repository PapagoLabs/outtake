// Copyright (c) 2026 - Nicholas Fedor <nick@nickfedor.com>
// SPDX-License-Identifier: AGPL-3.0-or-later

package flags

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PapagoLabs/outtake/internal/settings/config"
)

func TestListenApply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give Listen
		want string
	}{
		{
			name: "an unset flag leaves the configured address",
			give: Listen{},
			want: "0.0.0.0:8080",
		},
		{
			name: "a passed flag overrides the configured address",
			give: Listen{Addr: "127.0.0.1:9090"},
			want: "127.0.0.1:9090",
		},
		{
			name: "a passed flag overrides a bare port",
			give: Listen{Addr: ":9090"},
			want: ":9090",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{ListenAddr: "0.0.0.0:8080"}

			test.give.Apply(cfg)

			assert.Equal(t, test.want, cfg.ListenAddr)
		})
	}
}

func TestListenBind(t *testing.T) {
	t.Parallel()

	listen := &Listen{}
	flagSet := pflag.NewFlagSet("server", pflag.ContinueOnError)

	listen.Bind(flagSet)

	flag := flagSet.Lookup(FlagListen)
	require.NotNil(t, flag)
	assert.Empty(t, flag.DefValue)
	assert.Equal(t, "Listen address, which overrides OUTTAKE_LISTEN_ADDR", flag.Usage)
	assert.Empty(t, listen.Addr)
}

func TestListenBindWritesTheParsedValue(t *testing.T) {
	t.Parallel()

	listen := &Listen{}
	flagSet := pflag.NewFlagSet("server", pflag.ContinueOnError)

	listen.Bind(flagSet)

	require.NoError(t, flagSet.Parse([]string{"--" + FlagListen, "127.0.0.1:9090"}))

	assert.Equal(t, "127.0.0.1:9090", listen.Addr)
}

func TestListenBindLeavesTheAddressEmptyWithoutTheFlag(t *testing.T) {
	t.Parallel()

	listen := &Listen{}
	cfg := &config.Config{ListenAddr: "0.0.0.0:8080"}
	flagSet := pflag.NewFlagSet("server", pflag.ContinueOnError)

	listen.Bind(flagSet)
	require.NoError(t, flagSet.Parse(nil))

	listen.Apply(cfg)

	assert.Empty(t, listen.Addr)
	assert.Equal(t, "0.0.0.0:8080", cfg.ListenAddr)
}
