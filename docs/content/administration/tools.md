---
title: Tools
description: "nmap on the server, and the switches that let transforms reach private addresses for a lab."
weight: 80
---

**Admin > Tools** lists what transforms use beyond gosint itself. Each tool
has its own settings, saved for every node.

## Web and network requests

Transforms fetching a site, a host or a whois server an entity names, and the
assistant's page lookups.

**May reach private addresses (loopback, private ranges, link-local), for a
lab.** Off by default. Off, a transform or the assistant cannot reach the
network the server sits in: a request to `127.0.0.1`, `10.0.0.0/8`,
`192.168.0.0/16`, link-local, CGNAT and similar ranges is refused when it
connects, which covers redirects and DNS answers pointing inward. Turn it on
to investigate a lab network (TryHackMe, HackTheBox, your own range) from a
server that sits in it.

## Nmap

The active source of the `ip_to_ports` transform, scanning the address itself.
Ports are found passively (Shodan InternetDB) by default, nmap only runs when
someone picks the active source.

- **Path.** An absolute path to the nmap binary. Empty looks `nmap` up on the
  server's `PATH`. The badge says whether **this node** finds it (**found**
  or **not found on this node**), and the hint names the binary it runs.
- **May reach private addresses.** The same switch as above, for nmap alone.

A server without nmap fails such a scan with a clear error. The container
image ships nmap.

## Transform file scripts

The scripts that [transform files](/extending/transform-files/) start with a
`run` line. They have no program path and no private address switch of their
own, their settings are the sandbox:

- **Run scripts in the sandbox.** On by default wherever this node can do
  it. The badge says whether it can: **available here** on Linux with
  Landlock (kernel 5.13 or newer, and a container runtime whose seccomp
  profile allows it), **not available on this node** elsewhere. Switched on
  where it is not available, scripts refuse to run with an error naming the
  setting. Switched off, they run as the server's own user, unconfined.
- **May also read.** Absolute paths, one per line, a script may read beyond
  the system and its own directory: a wordlist, a GeoIP database, an
  interpreter installed under a home directory.
- **May also write.** Absolute paths, one per line, a script may write to
  beyond its scratch directory.

What a confined script may reach by default is listed under
[the sandbox](/extending/transform-files/#the-sandbox).

## What stays refused

The cloud metadata addresses (`169.254.169.254`, `fd00:ec2::254` and
`100.100.100.200`) stay refused for web requests of the transforms and the
assistant even with the private address switch on, so a sketch cannot read
the server's cloud credentials.

## Several nodes

The settings are shared, the binaries are not. Every node of a team install
needs nmap installed at the path given, or on its `PATH`, and the same paths
for the scripts. The badges on this page only speak for the node answering.
