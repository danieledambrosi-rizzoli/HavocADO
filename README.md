# HavocADO

A customized fork of the [Havoc C2 Framework](https://github.com/HavocFramework/Havoc), extended with custom **Model Context Protocol (MCP)** integrations and Demon agent command tooling.

> **Note:** This project was developed for a school project and is unlikely to be actively maintained anytime soon.

## Overview & Key Changes

This fork focuses on bridging the Havoc server and Demon agents with AI-assisted workflows and custom payload generation:

* **Stability Fixes:** Resolved DNS hang issues affecting Demon agent connectivity.
* **MCP Server Integration:** Added an MCP server interface to interact with and automate Havoc operations.
* **Demon Command Tools:** Implemented custom command tooling for Demon agents via MCP (`mcp-demon-command-tools`).
* **Payload Generation:** Added automated payload generation capabilities (`mcp-generate-payload`).
* **Custom DNS Support:** Introduced DNS communication support for the Demon agent.

## Disclaimer & Credits

* **Upstream Project:** All core C2 architecture and original credit belong to [C5pider](https://github.com/Cracked5pider) and the [Havoc Framework](https://github.com/HavocFramework/Havoc) contributors.
* **Testing & Sideloading:** Special shoutout to [Nuf1p](https://github.com/Nuf1p) and the [LoaderVille](https://github.com/Nuf1p/LoaderVille) project—in particular the [`fzsftp` sideloader](https://github.com/Nuf1p/LoaderVille/tree/main/Sideloader/fzsftp), which was used during testing.
* **AI-Assisted Development:** Parts of the refactoring, MCP integration, and debugging in this fork were developed with the assistance of AI tools.
* **Authorized Use Only:** This project is intended strictly for educational purposes, authorized security assessments, and research. Do not use this software on systems without explicit permission.
