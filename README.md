# go-ssdp

[![test](https://img.shields.io/github/actions/workflow/status/lrnxzz/go-ssdp/test.yml?branch=main&label=test)](https://github.com/lrnxzz/go-ssdp/actions/workflows/test.yml)
[![coverage](https://raw.githubusercontent.com/lrnxzz/go-ssdp/badges/.badges/main/coverage.svg)](.testcoverage.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/lrnxzz/go-ssdp/ssdp.svg)](https://pkg.go.dev/github.com/lrnxzz/go-ssdp/ssdp)
[![release](https://img.shields.io/github/v/release/lrnxzz/go-ssdp)](https://github.com/lrnxzz/go-ssdp/releases/latest)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

go-ssdp is a Go implementation of the Simple Service Discovery Protocol, the discovery layer of UPnP.
It follows chapter 1 of the [UPnP Device Architecture 1.1](https://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v1.1.pdf)
and [2.0](https://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v2.0.pdf), and the IETF draft
[SSDP/1.0](https://datatracker.ietf.org/doc/html/draft-cai-ssdp-v1-03) they build on.

It covers both sides of the protocol. As a control point, it searches the network over multicast or
unicast and keeps the cache of devices the specification asks for, following them as they appear,
reboot and leave. As a device, it announces itself, answers searches and cancels its announcements
when it goes away. Every message is read and written by a codec that checks it against the rules of
the version it declares.

Each test run starts real SSDP implementations in Docker, Gerbera, MiniDLNA and async-upnp-client,
and holds the library to what they send and accept.

Runnable programs are in [`ssdp/examples`](ssdp/examples), and [CONTRIBUTING.md](CONTRIBUTING.md)
explains how to work on the project.
