# -*- coding: utf-8 -*-
"""
自动同步模块 - 爬虫完成后自动触发后端数据同步
"""

import os
import requests
from typing import Optional


class AutoSync:
    """自动同步到后端 articles 表"""

    def __init__(self):
        self.backend_url = os.getenv("BACKEND_URL", "http://localhost:8080")
        self.backend_token = os.getenv("BACKEND_TOKEN", "")
        self.enabled = os.getenv("AUTO_SYNC_ENABLED", "true").lower() == "true"

    def trigger_sync(self, platform: str, sync_mode: str = "incremental") -> Optional[dict]:
        """
        触发后端同步

        Args:
            platform: 平台标识 (xhs/dy/bili/wb/ks/tieba/zhihu)
            sync_mode: 同步模式 (incremental/full)

        Returns:
            同步结果字典，失败返回 None
        """
        if not self.enabled:
            print(f"[AutoSync] 自动同步已禁用，跳过")
            return None

        print(f"[AutoSync] 开始同步 {platform} 数据到 articles 表...")

        try:
            headers = {
                "Content-Type": "application/json"
            }

            # 如果有 token，添加到请求头
            if self.backend_token:
                headers["Authorization"] = f"Bearer {self.backend_token}"

            response = requests.post(
                f"{self.backend_url}/api/platform/sync",
                json={
                    "platforms": [platform]  # 新 API 使用 platforms 数组
                },
                headers=headers,
                timeout=30  # 30秒超时
            )

            if response.status_code == 200:
                result = response.json()
                data = result.get("data", {})

                # 新 API 返回的是 map[platform]result
                platform_result = data.get(platform, {})

                print(f"✅ [AutoSync] {platform} 同步完成:")
                print(f"   - 总数据量: {platform_result.get('totalCount', 0)}")
                print(f"   - 新增: {platform_result.get('newCount', 0)}")
                print(f"   - 跳过: {platform_result.get('skippedCount', 0)}")
                print(f"   - 错误: {platform_result.get('errorCount', 0)}")
                print(f"   - 耗时: {platform_result.get('duration', 'N/A')}")

                return platform_result
            else:
                print(f"⚠️ [AutoSync] {platform} 同步失败: HTTP {response.status_code}")
                print(f"   响应: {response.text}")
                return None

        except requests.exceptions.Timeout:
            print(f"❌ [AutoSync] {platform} 同步超时（30秒）")
            return None
        except requests.exceptions.ConnectionError:
            print(f"❌ [AutoSync] {platform} 无法连接到后端服务 ({self.backend_url})")
            print(f"   提示: 请确保后端服务已启动")
            return None
        except Exception as e:
            print(f"❌ [AutoSync] {platform} 同步异常: {e}")
            return None

    def get_sync_status(self) -> Optional[dict]:
        """获取所有平台的同步状态"""
        if not self.enabled:
            return None

        try:
            headers = {}
            if self.backend_token:
                headers["Authorization"] = f"Bearer {self.backend_token}"

            response = requests.get(
                f"{self.backend_url}/api/platform/sync/status",
                headers=headers,
                timeout=10
            )

            if response.status_code == 200:
                result = response.json()
                return result.get("data", {})
            else:
                return None

        except Exception as e:
            print(f"[AutoSync] 获取同步状态失败: {e}")
            return None


# 全局实例
auto_sync = AutoSync()


def trigger_sync_after_crawl(platform: str):
    """
    爬虫完成后调用此函数自动触发同步

    Args:
        platform: 平台标识 (xhs/dy/bili/wb/ks/tieba/zhihu)
    """
    return auto_sync.trigger_sync(platform)
