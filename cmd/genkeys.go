// Copyright © 2016 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package cmd

import (
	"github.com/TheThingsNetwork/ttn/utils/security"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func genKeypairCmd(component string) *cobra.Command {
	return &cobra.Command{
		Use:   "gen-keypair",
		Short: "Generate a public/private keypair",
		Long:  `ttn gen-keypair generates a public/private keypair`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := security.GenerateKeypair(viper.GetString("key-dir")); err != nil {
				ctx.WithError(err).Fatal("Could not generate keypair")
			}
			ctx.WithField("TLSDir", viper.GetString("key-dir")).Info("Done")
		},
	}
}

func genCertCmd(component string) *cobra.Command {
	return &cobra.Command{
		Use:   "gen-cert",
		Short: "Generate a TLS certificate",
		Long:  `ttn gen-cert generates a TLS Certificate`,
		Run: func(cmd *cobra.Command, args []string) {
			var names []string
			if announcedName := viper.GetString(component + ".server-address-announce"); announcedName != "" {
				names = append(names, announcedName)
			}
			names = append(names, args...)
			if err := security.GenerateCert(viper.GetString("key-dir"), names...); err != nil {
				ctx.WithError(err).Fatal("Could not generate certificate")
			}
			ctx.WithField("TLSDir", viper.GetString("key-dir")).Info("Done")
		},
	}
}

func init() {
	routerCmd.AddCommand(genKeypairCmd("router"))
	brokerCmd.AddCommand(genKeypairCmd("broker"))
	handlerCmd.AddCommand(genKeypairCmd("handler"))
	discoveryCmd.AddCommand(genKeypairCmd("discovery"))
	networkserverCmd.AddCommand(genKeypairCmd("networkserver"))

	routerCmd.AddCommand(genCertCmd("router"))
	brokerCmd.AddCommand(genCertCmd("broker"))
	handlerCmd.AddCommand(genCertCmd("handler"))
	discoveryCmd.AddCommand(genCertCmd("discovery"))
	networkserverCmd.AddCommand(genCertCmd("networkserver"))
}
