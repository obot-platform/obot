---
title: Quick Start
slug: /installation/quickstart
---

**Intro**: Get Obot up and running locally on your machine in minutes.

## Requirements

- Docker installed
- Laptop specs: 2GB RAM, 10GB disk (4GB/40GB recommended)

## 1. Start Obot Docker Container

```bash
docker run -d \
  -p 8080:8080 \
  -v obot-data:/data \
  ghcr.io/obot-platform/obot:latest
```

## 2. Access Web Admin Interface

Open browser and visit: `http://localhost:8080/admin/agents`

## 3. Configure Model Provider

1. Go to **Model Providers**
2. Add API credentials for your LLM provider
3. Save configuration
4. Set default models for each category

## Chat With Obot

1. Go to `http://localhost:8080/`
1. Click `+ New Obot`.
1. You can start chatting with the obot.

## Next Steps

- [Building An Obot]
- [Check the Installation Guide](/installation/overview)  
