# typed: false
# frozen_string_literal: true

# This file was generated by GoReleaser. DO NOT EDIT.
class Rageta < Formula
  desc "Cloud native pipelines"
  homepage "https://github.com/raffis/rageta"
  version "0.0.2"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/raffis/rageta/releases/download/v0.0.2/rageta_0.0.2_darwin_amd64.tar.gz"
      sha256 "c9e3d02ad5f7f059781f75321b4200f372bbbe2de98528127b933d4653a2f5ea"

      def install
        bin.install "rageta"
      end
    end
    if Hardware::CPU.arm?
      url "https://github.com/raffis/rageta/releases/download/v0.0.2/rageta_0.0.2_darwin_arm64.tar.gz"
      sha256 "e5541abd882cb27a68e08e5e23203cbe7ed377acce9d68bca94ea0b74599888c"

      def install
        bin.install "rageta"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      if Hardware::CPU.is_64_bit?
        url "https://github.com/raffis/rageta/releases/download/v0.0.2/rageta_0.0.2_linux_amd64.tar.gz"
        sha256 "ae7e5be5df20802b500298fdb934a0dd54a1dd3dd26aa24a84a094e6ad07a5ad"

        def install
          bin.install "rageta"
        end
      end
    end
    if Hardware::CPU.arm?
      if Hardware::CPU.is_64_bit?
        url "https://github.com/raffis/rageta/releases/download/v0.0.2/rageta_0.0.2_linux_arm64.tar.gz"
        sha256 "113682b57de09da8075abe1cec04e3493562a5df0b750d391f7e71743a68af26"

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
