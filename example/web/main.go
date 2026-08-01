// Package main implements a pure Go web showcase and dashboard for WhatsRook using htmlbuilder.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Thruqe/htmlbuilder"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, renderDashboardPage())
	})

	log.Printf("WhatsRook Web Showcase running on http://localhost:%s", port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func renderDashboardPage() string {
	doc := htmlbuilder.New().
		Title("WhatsRook — Next-Gen WhatsApp Automation Bot").
		MetaDefault().
		Link(map[string]string{
			"rel":  "stylesheet",
			"href": "https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&display=swap",
		}).
		Link(map[string]string{
			"rel":  "stylesheet",
			"href": "https://cdn-uicons.flaticon.com/2.6.0/uicons-regular-rounded/css/uicons-regular-rounded.css",
		}).
		StyleBlock(`
			* {
				box-sizing: border-box;
			}
			html {
				scroll-behavior: smooth;
			}
			body {
				margin: 0;
				padding: 0;
				font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, sans-serif;
				background: #090d16;
				color: #f8fafc;
				overflow-x: hidden;
			}
			.glass-card {
				background: rgba(15, 23, 42, 0.7);
				backdrop-filter: blur(16px);
				-webkit-backdrop-filter: blur(16px);
				border: 1px solid rgba(255, 255, 255, 0.08);
				box-shadow: 0 20px 50px rgba(0, 0, 0, 0.4);
			}
			.glow-cyan {
				box-shadow: 0 0 30px rgba(6, 182, 212, 0.25);
			}
			.glow-indigo {
				box-shadow: 0 0 30px rgba(99, 102, 241, 0.25);
			}
			.gradient-text {
				background: linear-gradient(135deg, #38bdf8 0%, #818cf8 50%, #c084fc 100%);
				-webkit-background-clip: text;
				-webkit-text-fill-color: transparent;
			}
			@keyframes pulseGlow {
				0%, 100% { opacity: 0.4; transform: scale(1); }
				50% { opacity: 0.8; transform: scale(1.05); }
			}
			.bg-blur-1 {
				position: absolute;
				top: -10%;
				left: 20%;
				width: 500px;
				height: 500px;
				background: radial-gradient(circle, rgba(99, 102, 241, 0.25) 0%, rgba(0,0,0,0) 70%);
				pointer-events: none;
				animation: pulseGlow 8s infinite ease-in-out;
			}
			.bg-blur-2 {
				position: absolute;
				top: 40%;
				right: 10%;
				width: 600px;
				height: 600px;
				background: radial-gradient(circle, rgba(6, 182, 212, 0.2) 0%, rgba(0,0,0,0) 70%);
				pointer-events: none;
				animation: pulseGlow 10s infinite ease-in-out;
			}
		`)

	// Navigation Bar
	navBar := htmlbuilder.El("nav").Child(
		htmlbuilder.El("div").CSS(htmlbuilder.Style{
			Display:    "flex",
			AlignItems: "center",
			Gap:        "0.75rem",
		}).Child(
			htmlbuilder.El("div").CSS(htmlbuilder.Style{
				Width:        "36px",
				Height:       "36px",
				BorderRadius: "10px",
				Background:   "linear-gradient(135deg, #6366f1, #06b6d4)",
				Display:      "flex",
				AlignItems:   "center",
				JustifyContent: "center",
				FontWeight:   "800",
				FontSize:     "1.2rem",
				Color:        "#ffffff",
			}).Child(htmlbuilder.Span("R")),
			htmlbuilder.Span("WhatsRook").CSS(htmlbuilder.Style{
				FontWeight: "800",
				FontSize:   "1.5rem",
				LetterSpacing: "-0.02em",
				Color:      "#ffffff",
			}),
		),
		htmlbuilder.El("div").CSS(htmlbuilder.Style{
			Display: "flex",
			Gap:     "2rem",
			AlignItems: "center",
		}).Child(
			htmlbuilder.A("Features").Attr("href", "#features").CSS(navLinkStyle()),
			htmlbuilder.A("Commands").Attr("href", "#commands").CSS(navLinkStyle()),
			htmlbuilder.A("Architecture").Attr("href", "#architecture").CSS(navLinkStyle()),
			htmlbuilder.A("GitHub").Attr("href", "https://github.com/Thruqe/whatsrook").Attr("target", "_blank").CSS(htmlbuilder.Style{
				Background:     "linear-gradient(135deg, #6366f1, #4f46e5)",
				Color:          "#ffffff",
				Padding:        "0.6rem 1.4rem",
				BorderRadius:   "10px",
				FontWeight:     "600",
				TextDecoration: "none",
				FontSize:       "0.95rem",
				Transition:     "all 0.2s ease",
			}).Hover(htmlbuilder.Style{
				Transform: "translateY(-2px)",
				BoxShadow: "0 8px 20px rgba(99, 102, 241, 0.4)",
			}),
		),
	).CSS(htmlbuilder.Style{
		Display:        "flex",
		JustifyContent: "space-between",
		AlignItems:     "center",
		Padding:        "1.25rem 3rem",
		MaxWidth:       "1300px",
		Margin:         "0 auto",
		Width:          "100%",
		Position:       "fixed",
		Top:            "0",
		Left:           "0",
		Right:          "0",
		ZIndex:         "1000",
		Background:     "rgba(9, 13, 22, 0.85)",
		BorderBottom:   "1px solid rgba(255, 255, 255, 0.08)",
	}).SetStyle("backdrop-filter", "blur(16px)").SetStyle("-webkit-backdrop-filter", "blur(16px)")

	// Hero Section
	heroSection := htmlbuilder.El("section").Child(
		htmlbuilder.El("div").Class("bg-blur-1"),
		htmlbuilder.El("div").Class("bg-blur-2"),
		htmlbuilder.El("div").CSS(htmlbuilder.Style{
			MaxWidth:  "900px",
			Margin:    "0 auto",
			TextAlign: "center",
			Position:  "relative",
			ZIndex:    "2",
		}).Child(
			htmlbuilder.El("div").CSS(htmlbuilder.Style{
				Display:       "inline-flex",
				AlignItems:    "center",
				Gap:           "0.5rem",
				Background:    "rgba(99, 102, 241, 0.12)",
				Border:        "1px solid rgba(99, 102, 241, 0.3)",
				Padding:       "0.4rem 1.2rem",
				BorderRadius:  "30px",
				MarginBottom:  "2rem",
				FontSize:      "0.9rem",
				FontWeight:    "600",
				Color:         "#a5b4fc",
			}).Child(
				htmlbuilder.Span("⚡ Pure Go Architecture • High Performance"),
			),
			htmlbuilder.H1("Next-Generation WhatsApp Automation Engine").
				Class("gradient-text").
				CSS(htmlbuilder.Style{
					FontSize:      "4.2rem",
					FontWeight:    "800",
					LineHeight:    "1.15",
					LetterSpacing: "-0.03em",
					Margin:        "0 0 1.5rem",
				}),
			htmlbuilder.P("WhatsRook is a high-speed, modular WhatsApp bot daemon built with Go, whatsmeow, SQLite storage, Meta AI integration, and native media processing.").
				CSS(htmlbuilder.Style{
					Color:      "#94a3b8",
					FontSize:   "1.25rem",
					LineHeight: "1.7",
					Margin:     "0 auto 3rem",
					MaxWidth:   "750px",
					FontWeight: "400",
				}),
			htmlbuilder.El("div").CSS(htmlbuilder.Style{
				Display:        "flex",
				Gap:            "1.25rem",
				JustifyContent: "center",
				AlignItems:     "center",
			}).Child(
				htmlbuilder.A("Explore Features").
					Attr("href", "#features").
					CSS(htmlbuilder.Style{
						Background:     "linear-gradient(135deg, #06b6d4, #087ea4)",
						Color:          "#ffffff",
						Padding:        "1rem 2.5rem",
						BorderRadius:   "12px",
						FontWeight:     "700",
						FontSize:       "1.05rem",
						TextDecoration: "none",
						Transition:     "all 0.2s ease",
						BoxShadow:      "0 10px 25px rgba(6, 182, 212, 0.3)",
					}).
					Hover(htmlbuilder.Style{
						Transform: "translateY(-3px)",
						BoxShadow: "0 15px 35px rgba(6, 182, 212, 0.45)",
					}),
				htmlbuilder.A("View Source").
					Attr("href", "https://github.com/Thruqe/whatsrook").
					Attr("target", "_blank").
					CSS(htmlbuilder.Style{
						Background:     "rgba(255, 255, 255, 0.05)",
						Border:         "1px solid rgba(255, 255, 255, 0.15)",
						Color:          "#ffffff",
						Padding:        "1rem 2.5rem",
						BorderRadius:   "12px",
						FontWeight:     "600",
						FontSize:       "1.05rem",
						TextDecoration: "none",
						Transition:     "all 0.2s ease",
					}).
					Hover(htmlbuilder.Style{
						Background:  "rgba(255, 255, 255, 0.1)",
						BorderColor: "rgba(255, 255, 255, 0.3)",
						Transform:   "translateY(-3px)",
					}),
			),
		),
	).CSS(htmlbuilder.Style{
		Position:       "relative",
		Padding:        "12rem 2rem 8rem",
		MinHeight:      "90vh",
		Display:        "flex",
		AlignItems:     "center",
		JustifyContent: "center",
	})

	// Core Features Section
	featuresList := []struct {
		icon  string
		title string
		desc  string
	}{
		{"fi-rr-bolt", "Concurrent Dispatcher", "Handles incoming WhatsApp events concurrently with low-memory footprint and fast SQLite query storage."},
		{"fi-rr-brain", "Meta AI Deep Context", "Integrates natively with Meta AI bot sessions for automated, context-aware question answering and command invocation."},
		{"fi-rr-magic-wand", "Media & Sticker Engine", "Custom FFmpeg processing for WebP stickers, MP4 video conversion, circular masks, and crop transformations."},
		{"fi-rr-shield-check", "Granular Security", "Multi-layered moderation, Sudoer authorization, anti-spam rate limiting, and banned user enforcement."},
		{"fi-rr-refresh", "Self-Updating Daemon", "Automatic release checks, binary updates from GitHub releases, and seamless zero-downtime process restarts."},
		{"fi-rr-spinner", "Animated Status Loader", "Live message editing with Braille frame spinners while background tasks execute."},
	}

	featuresSection := htmlbuilder.El("section").
		Attr("id", "features").
		Child(
			htmlbuilder.El("div").CSS(htmlbuilder.Style{
				MaxWidth: "1200px",
				Margin:   "0 auto",
			}).Child(
				htmlbuilder.El("div").CSS(htmlbuilder.Style{
					TextAlign:    "center",
					MarginBottom: "4rem",
				}).Child(
					htmlbuilder.H2("Built for Performance & Scale").
						Class("gradient-text").
						CSS(htmlbuilder.Style{
							FontSize:      "2.75rem",
							FontWeight:    "800",
							Margin:        "0 0 1rem",
							LetterSpacing: "-0.02em",
						}),
					htmlbuilder.P("Everything you need for an enterprise-grade WhatsApp assistant, built 100% in Go.").
						CSS(htmlbuilder.Style{
							Color:    "#94a3b8",
							FontSize: "1.15rem",
							Margin:   "0",
						}),
				),
				htmlbuilder.El("div").CSS(htmlbuilder.Style{
					Display:             "grid",
					GridTemplateColumns: "repeat(auto-fit, minmax(340px, 1fr))",
					Gap:                 "2rem",
				}).Child(
					htmlbuilder.Each(featuresList, func(f struct {
						icon  string
						title string
						desc  string
					}) *htmlbuilder.Node {
						return htmlbuilder.El("div").
							Class("glass-card").
							CSS(htmlbuilder.Style{
								Padding:      "2.5rem",
								BorderRadius: "20px",
								Transition:   "all 0.3s cubic-bezier(0.4, 0, 0.2, 1)",
							}).
							Hover(htmlbuilder.Style{
								Transform:   "translateY(-6px)",
								BorderColor: "rgba(99, 102, 241, 0.4)",
								BoxShadow:   "0 20px 40px rgba(99, 102, 241, 0.2)",
							}).
							Child(
								htmlbuilder.El("div").CSS(htmlbuilder.Style{
									Width:          "50px",
									Height:         "50px",
									BorderRadius:   "14px",
									Background:     "rgba(99, 102, 241, 0.15)",
									Border:         "1px solid rgba(99, 102, 241, 0.3)",
									Display:        "flex",
									AlignItems:     "center",
									JustifyContent: "center",
									MarginBottom:   "1.5rem",
									Color:          "#818cf8",
									FontSize:       "1.4rem",
								}).Child(
									htmlbuilder.El("i").Class("fi", f.icon),
								),
								htmlbuilder.H3(f.title).CSS(htmlbuilder.Style{
									FontSize:     "1.35rem",
									FontWeight:   "700",
									Margin:       "0 0 1rem",
									Color:        "#ffffff",
								}),
								htmlbuilder.P(f.desc).CSS(htmlbuilder.Style{
									Color:      "#94a3b8",
									FontSize:   "1rem",
									LineHeight: "1.6",
									Margin:     "0",
								}),
							)
					})...,
				),
			),
		).CSS(htmlbuilder.Style{
		Padding: "6rem 2rem",
	})

	// Architecture Showcase Section
	archSection := htmlbuilder.El("section").
		Attr("id", "architecture").
		Child(
			htmlbuilder.El("div").CSS(htmlbuilder.Style{
				MaxWidth: "1100px",
				Margin:   "0 auto",
			}).Child(
				htmlbuilder.El("div").Class("glass-card", "glow-indigo").CSS(htmlbuilder.Style{
					Padding:      "3.5rem",
					BorderRadius: "28px",
					Background:   "linear-gradient(135deg, rgba(15, 23, 42, 0.9), rgba(30, 41, 59, 0.8))",
				}).Child(
					htmlbuilder.El("div").CSS(htmlbuilder.Style{
						Display:             "grid",
						GridTemplateColumns: "1fr 1fr",
						Gap:                 "3rem",
						AlignItems:          "center",
					}).Child(
						htmlbuilder.El("div").Child(
							htmlbuilder.H2("Constructed with htmlbuilder").
								Class("gradient-text").
								CSS(htmlbuilder.Style{
									FontSize:   "2.25rem",
									FontWeight: "800",
									Margin:     "0 0 1.25rem",
								}),
							htmlbuilder.P("This entire web showcase is written in pure Go using github.com/Thruqe/htmlbuilder. No raw HTML templates, no Node.js dependencies, no Javascript frameworks — compiled straight into the Go binary.").
								CSS(htmlbuilder.Style{
									Color:      "#cbd5e1",
									FontSize:   "1.1rem",
									LineHeight: "1.7",
									Margin:     "0 0 2rem",
								}),
							htmlbuilder.El("div").CSS(htmlbuilder.Style{
								Display: "flex",
								Gap:     "1rem",
							}).Child(
								htmlbuilder.El("div").CSS(htmlbuilder.Style{
									Background:   "rgba(6, 182, 212, 0.1)",
									Border:       "1px solid rgba(6, 182, 212, 0.3)",
									Padding:      "1rem 1.5rem",
									BorderRadius: "12px",
									TextAlign:    "center",
								}).Child(
									htmlbuilder.Span("100%").CSS(htmlbuilder.Style{
										Display:    "block",
										FontSize:   "1.8rem",
										FontWeight: "800",
										Color:      "#38bdf8",
									}),
									htmlbuilder.Span("Pure Go Code").CSS(htmlbuilder.Style{
										FontSize: "0.85rem",
										Color:    "#94a3b8",
									}),
								),
								htmlbuilder.El("div").CSS(htmlbuilder.Style{
									Background:   "rgba(99, 102, 241, 0.1)",
									Border:       "1px solid rgba(99, 102, 241, 0.3)",
									Padding:      "1rem 1.5rem",
									BorderRadius: "12px",
									TextAlign:    "center",
								}).Child(
									htmlbuilder.Span("0").CSS(htmlbuilder.Style{
										Display:    "block",
										FontSize:   "1.8rem",
										FontWeight: "800",
										Color:      "#a5b4fc",
									}),
									htmlbuilder.Span("External JS/CSS Files").CSS(htmlbuilder.Style{
										FontSize: "0.85rem",
										Color:    "#94a3b8",
									}),
								),
							),
						),
						htmlbuilder.El("div").CSS(htmlbuilder.Style{
							Background:   "#020617",
							Border:       "1px solid rgba(255, 255, 255, 0.1)",
							BorderRadius: "16px",
							Padding:      "1.75rem",
							FontFamily:   "monospace",
							FontSize:     "0.9rem",
							Color:        "#38bdf8",
							LineHeight:   "1.6",
							OverflowX:    "auto",
						}).Child(
							htmlbuilder.Span("// Build UI declaratively in Go\n"),
							htmlbuilder.Span("doc := htmlbuilder.New()\n"),
							htmlbuilder.Span("doc.Title(\"WhatsRook Showcase\")\n"),
							htmlbuilder.Span("doc.Body().Child(\n"),
							htmlbuilder.Span("    htmlbuilder.H1(\"Pure Go Engine\"),\n"),
							htmlbuilder.Span("    htmlbuilder.P(\"Type-safe Web UI\"),\n"),
							htmlbuilder.Span(")\n\n"),
							htmlbuilder.Span("fmt.Fprintln(w, doc.String())"),
						),
					),
				),
			),
		).CSS(htmlbuilder.Style{
		Padding: "4rem 2rem 6rem",
	})

	// Footer
	footer := htmlbuilder.El("footer").Child(
		htmlbuilder.El("div").CSS(htmlbuilder.Style{
			MaxWidth:       "1200px",
			Margin:         "0 auto",
			Display:        "flex",
			JustifyContent: "space-between",
			AlignItems:     "center",
		}).Child(
			htmlbuilder.P("© 2026 WhatsRook Project. Powered by Go & htmlbuilder.").CSS(htmlbuilder.Style{
				Color:      "#64748b",
				FontSize:   "0.95rem",
				Margin:     "0",
				FontWeight: "500",
			}),
			htmlbuilder.A("GitHub Repository").
				Attr("href", "https://github.com/Thruqe/whatsrook").
				Attr("target", "_blank").
				CSS(htmlbuilder.Style{
					Color:          "#38bdf8",
					TextDecoration: "none",
					FontWeight:     "600",
					FontSize:       "0.95rem",
				}),
		),
	).CSS(htmlbuilder.Style{
		Padding:      "3rem 2rem",
		BorderTop:    "1px solid rgba(255, 255, 255, 0.08)",
		Background:   "#060911",
		BoxSizing:    "border-box",
	})

	// Build Full Document Structure
	doc.Body().Child(navBar, heroSection, featuresSection, archSection, footer)

	return doc.String()
}

func navLinkStyle() htmlbuilder.Style {
	return htmlbuilder.Style{
		Color:          "#94a3b8",
		TextDecoration: "none",
		FontWeight:     "600",
		FontSize:       "0.95rem",
		Transition:     "color 0.2s ease",
	}
}
