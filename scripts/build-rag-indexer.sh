#!/usr/bin/env bash
set -e
source /etc/profile.d/mathwiki-go.sh
cd /opt/mathwiki/backend
GOPROXY=https://goproxy.cn,direct GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 \
  go build -v -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-rag-indexer.new ./cmd/rag-indexer
chmod 755 bin/mathwiki-rag-indexer.new
mv -f bin/mathwiki-rag-indexer.new bin/mathwiki-rag-indexer
printf '全库建库工具更新完成\n'
