---
title: Architecture
---

## Overview

The Obot platform is designed to enable users to consume MCP servers in an enterprise setting. It consists of three main components that work together to provide a complete solution: the MCP Gateway for tool integration, the Chat Interface for user interaction, and the Admin Interface for platform management.

![ALT Text](/img/high-level-arch.png)

## Concepts

- **MCP Clients:** MCP clients are the tools that interact with LLMs and consume MCP servers. These clients can be agents, desktop tools like Cursor, Claude, VS Code, or any other applications that interface with LLMs. They are responsible for handling the interactions with MCP servers, LLMs, and the end user.

- **MCP Servers:** MCP servers are the bits of code that implement the MCP spec like tools, prompts, and resources. It serves them up to be consumed by the clients to connect with LLMs.

- **MCP Catalog:** The catalog is an index of MCP servers that contains metadata about them, including how to run them and where to get them. It serves as a centralized repository for discovering and managing available MCP servers.

- **MCP Gateway:** The gateway is a proxy that sits between the client and the server, providing important capabilities for enterprise deployment. It performs auditing, inspects payloads, and applies policy and access controls to MCP servers. This is a critical piece of the tool and capability infrastructure.

- **LLM Gateway:** The LLM gateway (or LLM proxy) is the connectivity layer that sits between a chat client and the LLM. It also provides proxy capabilities to the interaction, enabling additional control and monitoring of LLM communications.

- **MCP Hosting:** The hosting infrastructure running the MCP server code.

## Authentication flows

All clients first authenticate with the Obot Gateway. The Obot Gateway is setup to authenticate with the organizations identity provider. Once the user is authenticated with the gateway, they will do additional OAUTH logins with the downstream MCP servers if they are configured to.

```text
                                                                                 
                                                                                 
                                                                                 
      ┌──────────────────────┐                                                   
      │                      │                                                   
      │        Client        │                                                   
      │                      │                                                   
      └──────────┬───────────┘                                                   
                 │                                                               
                 │  Authenticated?                                               
                 │                                                               
                 │                                                               
      ┌──────────▼───────────┐                     ┌──────────────────┐          
      │                      │       No            │                  │          
      │         IDP          ┼─────────────────────►   Authenticate   │          
      │                      │                     │                  │          
      └──────────┬───────────┘                     └──────────────────┘          
                 │                                                               
                 │ Yes                                                           
                 │                                                               
      ┌──────────▼───────────┐                                                   
      │                      │                                                   
      │     Obot Gateway     │                                                   
      │                      │                                                   
      └──────────┬───────────┘                                                   
                 │                                                               
                 │                                                               
                 │  Authorized?                                                  
                 │                                                               
                 │                                                               
      ┌──────────▼───────────┐                    ┌───────────────────┐          
      │                      │                    │                   │          
      │     MCP Server       ┼────────────────────►     MCP Auth      │          
      │                      │                    │                   │          
      └──────────────────────┘                    └───────────────────┘          
```

## Obot Server

```text
                        ┌────────────────┐                                                         
                        │   MCP Clients  │                                                         
                        └───────┬────────┘                                                         
                                │                                                                  
                                │                                                                  
                                │                                                                  
                      ┌─────────────────────────────────────────────────────────┐                  
                      │ ┌───────▼──────┐                                        │                  
                      │ │              │                          Kubernetes    │                  
                      │ │   Obot GW    │                                        │                  
                  ┌───│─┼              │                                        │                  
                  │   │ └────────┬─────┴────────────┬───────────────┐           │                  
                  │   │          │                  │               │           │                  
                  │   │          │                  │               │           │                  
                  │   │          │                  │               │           │                  
                  │   │    ┌─────▼────────┐ ┌───────▼──────┐ ┌──────▼───────┐   │                  
                  │   │    │  MCP Server  │ │  MCP Server  │ │  MCP Server  │   │                  
                  │   │    └─────────┬────┘ └───────────┬──┘ └──────────────┘   │                  
                  │   │              │                  │                       │                  
                  │   │              │                  │                       │                  
                  │   │              │                  │                       │                  
                  │   └──────────────┼──────────────────┼───────────────────────┘                  
                  │                  │                  ▼                                          
         ┌────────▼───────┐    ┌─────▼───────────┐    ┌───────────────────┐                        
         │                │    │                 │    │                   │                        
         │   Remote MCP   │    │    Databases    │    │     SaaS APIs     │                        
         │                │    │                 │    │                   │                        
         └────────────────┘    └─────────────────┘    └───────────────────┘                        
```

### Data persistence

Data is stored in a Postgres Database. In production, this should be hosted and managed independently of the Obot Gateway deployment.

Obot chat uses an S3 Bucket to store workspace data, like PDFs, text files, etc.


### Encryption

Obot Gateway uses cloud KMS systems to encrypt data at rest. 

### LLMs

Obot operates with a bring your own model philosophy. There are many providers that can be configured to meet your organizations requirements.