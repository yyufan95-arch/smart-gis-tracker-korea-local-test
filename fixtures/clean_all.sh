#!/bin/bash

# 强制删除所有容器（如果有）
containers=$(docker ps -aq)
if [ -n "$containers" ]; then
  docker rm -f $containers
else
  echo "✅ 没有需要删除的容器"
fi

# 删除所有 volumes
volumes=$(docker volume ls -q)
if [ -n "$volumes" ]; then
  docker volume rm $volumes
else
  echo "✅ 没有需要删除的 volumes"
fi

# 删除所有自定义 network（排除 bridge/host/none）
networks=$(docker network ls --filter "driver=bridge" --format "{{.Name}}" | grep -vE "bridge|host|none")
if [ -n "$networks" ]; then
  docker network rm $networks
else
  echo "✅ 没有需要删除的自定义网络"
fi

