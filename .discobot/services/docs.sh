#!/bin/bash
#---
# name: Docs
# description: Docusaurus docs dev server
# http: 3000
#---
cd /home/discobot/workspace/docs
npm install --silent
npx docusaurus start --host 0.0.0.0 --port 3000
