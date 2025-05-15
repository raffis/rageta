---
sidebar_position: 1
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Introduction

Let's discover **Rageta in less than 5 minutes**.

## Getting Started

Get started by **creating a new site**.

Or **try Docusaurus immediately** with **[docusaurus.new](https://docusaurus.new)**.

### What you'll need

To run rageta pipelines you will need a container runtime. Currently only docker is supported.
Moreover you need to install the rageta cli:

<Tabs
  defaultValue="curl"
  values={[
    {label: 'Homebrew', value: 'homebrew'},
    {label: 'Curl', value: 'curl'}
  ]}>
  <TabItem value="homebrew">This is an apple 🍎</TabItem>
  <TabItem value="curl">This is an orange 🍊</TabItem>
</Tabs>

## Execute pipeline

The classic template will automatically be added to your project after you run the command:

```bash
rageta run ghcr.io/rageta/examples/hello-world:v1
```
