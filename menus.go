package main

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/neuralinkcorp/tsui/clipboard"
	"github.com/neuralinkcorp/tsui/libts"
	"github.com/neuralinkcorp/tsui/ui"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/types/opt"
	"tailscale.com/types/preftype"
)

func buildNetworkDevicesSubmenuSection(title string, peers []*ipnstate.PeerStatus) []ui.SubmenuItem {
	items := []ui.SubmenuItem{
		&ui.TitleSubmenuItem{Label: title},
	}

	if len(peers) == 0 {
		items = append(items, &ui.DividerSubmenuItem{})
	} else {
		for _, peer := range peers {
			peerName := libts.PeerName(peer)

			// Normalize the capitalization of the OS, because some are capitalized but some aren't.
			osName := peer.OS
			switch osName {
			case "android":
				osName = "Android"
			case "windows":
				osName = "Windows"
			case "linux":
				osName = "Linux"
			}

			items = append(items, &ui.LabeledSubmenuItem{
				Label:           peerName,
				AdditionalLabel: osName,
				OnActivate: func() tea.Msg {
					err := clipboard.WriteString(peer.TailscaleIPs[0].String())
					if err != nil {
						return errorMsg(err)
					}
					return successMsg(fmt.Sprintf("Copied IP address of %s.", peerName))
				},
				IsDim: false,
			})
		}
	}

	return items
}

// Update all of the menu UIs from the current state.
func (m *model) updateMenus() {
	if m.state.BackendState == ipn.Running {
		// Update the device info submenu.
		{
			submenuItems := []ui.SubmenuItem{
				&ui.TitleSubmenuItem{Label: "Name"},
				&ui.LabeledSubmenuItem{
					Label: m.state.Self.DNSName[:len(m.state.Self.DNSName)-1], // Remove the trailing dot.
					OnActivate: func() tea.Msg {
						err := clipboard.WriteString(m.state.Self.DNSName[:len(m.state.Self.DNSName)-1])
						if err != nil {
							return errorMsg(err)
						}
						return successMsg("Copied full domain to clipboard.")
					},
				},
				&ui.SpacerSubmenuItem{},
				&ui.TitleSubmenuItem{Label: "IPs"},
			}

			for _, addr := range m.state.Self.TailscaleIPs {
				submenuItems = append(submenuItems, &ui.LabeledSubmenuItem{
					Label: addr.String(),
					OnActivate: func() tea.Msg {
						err := clipboard.WriteString(addr.String())
						if err != nil {
							return errorMsg(err)
						}

						var versionName string
						if addr.Is4() {
							versionName = "IPv4"
						} else {
							versionName = "IPv6"
						}

						return successMsg(fmt.Sprintf("Copied %s address to clipboard.", versionName))
					},
				})
			}

			submenuItems = append(submenuItems,
				&ui.SpacerSubmenuItem{},
				&ui.TitleSubmenuItem{Label: "Debug Info"},
				&ui.LabeledSubmenuItem{
					Label: fmt.Sprintf("ID: %s", m.state.Self.ID),
					OnActivate: func() tea.Msg {
						err := clipboard.WriteString(string(m.state.Self.ID))
						if err != nil {
							return errorMsg(err)
						}
						return successMsg("Copied Tailscale node ID to clipboard.")
					},
				},
				&ui.LabeledSubmenuItem{
					Label: m.state.Self.PublicKey.String(),
					OnActivate: func() tea.Msg {
						err := clipboard.WriteString(m.state.Self.PublicKey.String())
						if err != nil {
							return errorMsg(err)
						}
						return successMsg("Copied node key to clipboard.")
					},
				},
			)

			if m.state.LockKey != nil {
				statusText := "Online"
				if m.state.IsLockedOut {
					statusText = "Locked Out"
				}

				submenuItems = append(submenuItems,
					&ui.SpacerSubmenuItem{},
					&ui.TitleSubmenuItem{Label: "Tailnet Lock: " + statusText},
					&ui.LabeledSubmenuItem{
						Label: m.state.LockKey.CLIString(),
						OnActivate: func() tea.Msg {
							err := clipboard.WriteString(m.state.LockKey.CLIString())
							if err != nil {
								return errorMsg(err)
							}
							return successMsg("Copied tailnet lock key to clipboard.")
						},
					},
				)
			}

			submenuItems = append(submenuItems,
				&ui.SpacerSubmenuItem{},
				&ui.LabeledSubmenuItem{
					Label:   "[Disconnect from Tailscale]",
					Variant: ui.SubmenuItemVariantAccent,
					OnActivate: func() tea.Msg {
						err := libts.Down(ctx)
						if err != nil {
							return errorMsg(err)
						}
						return tipMsg("You can also simply press . to disconnect.")
					},
				},
			)

			m.deviceInfo.Submenu.SetItems(submenuItems)
		}

		// Update the exit node submenu.
		{
			exitNodeItems := make([]ui.SubmenuItem, 0)

			// "None" option
			exitNodeItems = append(exitNodeItems, &ui.ToggleableSubmenuItem{
				LabeledSubmenuItem: ui.LabeledSubmenuItem{
					Label: "None",
					OnActivate: func() tea.Msg {
						err := libts.SetExitNode(ctx, nil)
						if err != nil {
							return errorMsg(err)
						}
						return updateState()
					},
				},
				IsActive: m.state.CurrentExitNode == nil,
			})

			exitNodeItems = append(exitNodeItems, &ui.DividerSubmenuItem{})

			// Tailnet exit nodes (non-Mullvad)
			if len(m.state.ExitNodes) > 0 {
				exitNodeItems = append(exitNodeItems, &ui.TitleSubmenuItem{Label: "Tailnet Exit Nodes"})
				for _, exitNode := range m.state.ExitNodes {
					pingLabel := "???"
					if !exitNode.Online {
						pingLabel = "Offline"
					} else if m.pings[exitNode.ID] != nil {
						pingLabel = fmt.Sprintf("%dms", int(math.Round(m.pings[exitNode.ID].LatencySeconds*1000)))
					}

					exitNodeItems = append(exitNodeItems, &ui.ToggleableSubmenuItem{
						LabeledSubmenuItem: ui.LabeledSubmenuItem{
							Label:           libts.PeerName(exitNode),
							AdditionalLabel: pingLabel,
							OnActivate: func() tea.Msg {
								err := libts.SetExitNode(ctx, exitNode)
								if err != nil {
									return errorMsg(err)
								}
								return updateState()
							},
							IsDim: !exitNode.Online,
						},
						IsActive: m.state.CurrentExitNode != nil && exitNode.ID == *m.state.CurrentExitNode,
					})
				}
			}

			// Mullvad exit nodes grouped by country/city
			if len(m.state.MullvadExitNodes) > 0 {
				if len(m.state.ExitNodes) > 0 {
					exitNodeItems = append(exitNodeItems, &ui.DividerSubmenuItem{})
				}

				type cityGroup struct {
					city  string
					nodes []*ipnstate.PeerStatus
				}
				countryGroups := make(map[string][]cityGroup)
				countryOrder := make([]string, 0)

				for _, node := range m.state.MullvadExitNodes {
					if node.Location == nil {
						continue
					}
					country := node.Location.Country
					city := node.Location.City

					if _, ok := countryGroups[country]; !ok {
						countryGroups[country] = make([]cityGroup, 0)
						countryOrder = append(countryOrder, country)
					}

					found := false
					for i, cg := range countryGroups[country] {
						if cg.city == city {
							countryGroups[country][i].nodes = append(countryGroups[country][i].nodes, node)
							found = true
							break
						}
					}
					if !found {
						countryGroups[country] = append(countryGroups[country], cityGroup{
							city:  city,
							nodes: []*ipnstate.PeerStatus{node},
						})
					}
				}

				slices.Sort(countryOrder)

				for i, country := range countryOrder {
					cities := countryGroups[country]
					if i > 0 {
						exitNodeItems = append(exitNodeItems, &ui.SpacerSubmenuItem{})
					}
					exitNodeItems = append(exitNodeItems, &ui.TitleSubmenuItem{Label: country})

					slices.SortFunc(cities, func(a, b cityGroup) int {
						return strings.Compare(a.city, b.city)
					})

					for _, cg := range cities {
						slices.SortFunc(cg.nodes, func(a, b *ipnstate.PeerStatus) int {
							return strings.Compare(libts.PeerName(a), libts.PeerName(b))
						})

						for _, node := range cg.nodes {
							pingLabel := "???"
							if !node.Online {
								pingLabel = "Offline"
							} else if m.pings[node.ID] != nil {
								pingLabel = fmt.Sprintf("%dms", int(math.Round(m.pings[node.ID].LatencySeconds*1000)))
							}

							label := cg.city
							if len(cg.nodes) > 1 {
								label = cg.city + " - " + libts.PeerName(node)
							}

							exitNodeItems = append(exitNodeItems, &ui.ToggleableSubmenuItem{
								LabeledSubmenuItem: ui.LabeledSubmenuItem{
									Label:           label,
									AdditionalLabel: pingLabel,
									OnActivate: func() tea.Msg {
										err := libts.SetExitNode(ctx, node)
										if err != nil {
											return errorMsg(err)
										}
										return updateState()
									},
									IsDim: !node.Online,
								},
								IsActive: m.state.CurrentExitNode != nil && node.ID == *m.state.CurrentExitNode,
							})
						}
					}
				}
			}

			m.exitNodes.AdditionalLabel = m.state.CurrentExitNodeName
			m.exitNodes.Submenu.SetItems(exitNodeItems)
		}

		// Update the network devices submenu.
		{
			networkNodes := make([]ui.SubmenuItem, 0)

			networkNodes = append(networkNodes,
				buildNetworkDevicesSubmenuSection("My Devices", m.state.MyNodes)...)
			networkNodes = append(networkNodes,
				&ui.SpacerSubmenuItem{})
			networkNodes = append(networkNodes,
				buildNetworkDevicesSubmenuSection("Tagged Devices", m.state.TaggedNodes)...)

			for _, key := range m.state.OwnedNodeKeys {
				if key == "" {
					key = "<none>"
				}

				networkNodes = append(networkNodes,
					&ui.SpacerSubmenuItem{})
				networkNodes = append(networkNodes,
					buildNetworkDevicesSubmenuSection(key, m.state.OwnedNodes[key])...)
			}

			lenSum := len(m.state.MyNodes) + len(m.state.TaggedNodes)
			for _, value := range m.state.OwnedNodes {
				lenSum += len(value)
			}

			m.networkDevices.AdditionalLabel = fmt.Sprintf("%d visible", lenSum)
			m.networkDevices.Submenu.SetItems(networkNodes)
		}

		// Update the settings submenu.
		{
			exitNode := "No"
			if m.state.Prefs.AdvertisesExitNode() {
				exitNode = "Exit Node"
			}

			accountTitle := "Account"
			reauthenticateButtonLabel := "[Reauthenticate]"
			if m.state.Self.KeyExpiry != nil {
				reauthenticateButtonLabel = "[Reauthenticate Now]"

				duration := time.Until(*m.state.Self.KeyExpiry)
				accountTitle += " - Key Expires in " + ui.FormatDuration(duration)
			}

			submenuItems := []ui.SubmenuItem{
				&ui.TitleSubmenuItem{Label: "General"},

				ui.NewYesNoSettingsSubmenuItem("Allow Incoming Connections",
					!m.state.Prefs.ShieldsUp,
					func(newValue bool) tea.Msg {
						return editPrefs(&ipn.MaskedPrefs{
							Prefs: ipn.Prefs{
								ShieldsUp: !newValue,
							},
							ShieldsUpSet: true,
						})
					},
				),

				ui.NewYesNoSettingsSubmenuItem("Use Subnet Routes",
					m.state.Prefs.RouteAll,
					func(newValue bool) tea.Msg {
						return editPrefs(&ipn.MaskedPrefs{
							Prefs: ipn.Prefs{
								RouteAll: newValue,
							},
							RouteAllSet: true,
						})
					},
				),

				ui.NewYesNoSettingsSubmenuItem("Use DNS Settings",
					m.state.Prefs.CorpDNS,
					func(newValue bool) tea.Msg {
						return editPrefs(&ipn.MaskedPrefs{
							Prefs: ipn.Prefs{
								CorpDNS: newValue,
							},
							CorpDNSSet: true,
						})
					},
				),

				&ui.SpacerSubmenuItem{},
				&ui.TitleSubmenuItem{Label: "Exit Nodes"},

				ui.NewYesNoSettingsSubmenuItem("Enable Local Network Access",
					m.state.Prefs.ExitNodeAllowLANAccess,
					func(newValue bool) tea.Msg {
						return editPrefs(&ipn.MaskedPrefs{
							Prefs: ipn.Prefs{
								ExitNodeAllowLANAccess: newValue,
							},
							ExitNodeAllowLANAccessSet: true,
						})
					},
				),

				ui.NewSettingsSubmenuItem("Advertise Exit Node",
					[]string{"Exit Node", "No"},
					exitNode,
					func(newLabel string) tea.Msg {
						var prefs ipn.Prefs
						prefs.SetAdvertiseExitNode(newLabel == "Exit Node")
						return editPrefs(&ipn.MaskedPrefs{
							Prefs:              prefs,
							AdvertiseRoutesSet: true,
						})
					},
				),

				&ui.SpacerSubmenuItem{},
				&ui.TitleSubmenuItem{Label: accountTitle},

				&ui.LabeledSubmenuItem{
					Label: reauthenticateButtonLabel,
					// Reauthenticating is basically the same as the first-time login flow.
					OnActivate: startLoginInteractive,
				},

				&ui.LabeledSubmenuItem{
					Label:   "[Log Out]",
					Variant: ui.SubmenuItemVariantDanger,
					OnActivate: func() tea.Msg {
						err := libts.Logout(ctx)
						if err != nil {
							return errorMsg(err)
						}
						return successMsg("Logged out.")
					},
				},
			}

			// On Linux, show the advanced Linux settings.
			if runtime.GOOS == "linux" {
				var netfilterMode string
				switch m.state.Prefs.NetfilterMode {
				case preftype.NetfilterOn:
					netfilterMode = "On"
				case preftype.NetfilterNoDivert:
					netfilterMode = "No Divert"
				case preftype.NetfilterOff:
					netfilterMode = "Off"
				}

				noStatefulFiltering, _ := m.state.Prefs.NoStatefulFiltering.Get()

				submenuItems = append(submenuItems,
					&ui.SpacerSubmenuItem{},
					&ui.TitleSubmenuItem{Label: "Advanced - Linux"},

					ui.NewSettingsSubmenuItem("NetFilter Mode",
						[]string{"On", "No Divert", "Off"},
						netfilterMode,
						func(newLabel string) tea.Msg {
							var netfilterMode preftype.NetfilterMode
							switch newLabel {
							case "On":
								netfilterMode = preftype.NetfilterOn
							case "No Divert":
								netfilterMode = preftype.NetfilterNoDivert
							case "Off":
								netfilterMode = preftype.NetfilterOff
							}

							return editPrefs(&ipn.MaskedPrefs{
								Prefs: ipn.Prefs{
									NetfilterMode: netfilterMode,
								},
								NetfilterModeSet: true,
							})
						},
					),

					ui.NewYesNoSettingsSubmenuItem("Enable Stateful Filtering",
						!noStatefulFiltering,
						func(newValue bool) tea.Msg {
							return editPrefs(&ipn.MaskedPrefs{
								Prefs: ipn.Prefs{
									NoStatefulFiltering: opt.NewBool(!newValue),
								},
								NoStatefulFilteringSet: true,
							})
						},
					),
				)
			}

			m.settings.Submenu.SetItems(submenuItems)
		}

		// Make sure the menu items are visible.
		m.menu.SetItems([]*ui.AppmenuItem{
			m.deviceInfo,
			m.exitNodes,
			m.networkDevices,
			m.settings,
		})
	} else {
		// Hide the menu items if not connected.
		// I mean, they won't be visible anyway, but extra safety is always nice!
		m.menu.SetItems([]*ui.AppmenuItem{})
	}
}
