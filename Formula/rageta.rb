# typed: false
# frozen_string_literal: true

# This file was generated by GoReleaser. DO NOT EDIT.
class Rageta < Formula
  desc "Cloud native pipelines"
  homepage "https://github.com/raffis/rageta"
  version "0.0.17"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/raffis/rageta/releases/download/v0.0.17/rageta_0.0.17_darwin_amd64.tar.gz"
      sha256 "5fdbdde598e892e69ee0c70d3881249a8d2c6bb077ee9fa28a6a3c0be6ce3f10"

      def install
        bin.install "rageta"
      end
    end
    if Hardware::CPU.arm?
      url "https://github.com/raffis/rageta/releases/download/v0.0.17/rageta_0.0.17_darwin_arm64.tar.gz"
      sha256 "c78d7069b7bed5550d30c2e478e874c5d902a96e3e684841046827ea4a1bb678"

      def install
        bin.install "rageta"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      if Hardware::CPU.is_64_bit?
        url "https://github.com/raffis/rageta/releases/download/v0.0.17/rageta_0.0.17_linux_amd64.tar.gz"
        sha256 "ca80a97ddc156c843bfb372eff2d7559f41b5f8a9c57febfc289b6c67f310a62"

        def install
          bin.install "rageta"
        end
      end
    end
    if Hardware::CPU.arm?
      if Hardware::CPU.is_64_bit?
        url "https://github.com/raffis/rageta/releases/download/v0.0.17/rageta_0.0.17_linux_arm64.tar.gz"
        sha256 "adeeb051ecc8a19d0386ba9325fb46b8cdc3e86e1a186abbfa3ff7c13585f5ad"

        def install
          bin.install "rageta"
        end
      end
    end
  end

  test do
    system "#{bin}/rageta -h"
  end
end
