# -*- coding: utf-8 -*-
import asyncio
import subprocess
import signal
import os
from typing import Optional, List, Dict
from datetime import datetime
from pathlib import Path

from ..schemas import CrawlerStartRequest, LogEntry


PLATFORM_NAMES: Dict[str, str] = {
    "xhs": "小红书",
    "dy": "抖音",
    "ks": "快手",
    "bili": "B站",
    "wb": "微博",
    "tieba": "百度贴吧",
    "zhihu": "知乎",
}

_CDP_PORT_MAP: Dict[str, int] = {
    "xhs": 9222,
    "dy": 9223,
    "ks": 9224,
    "bili": 9225,
    "wb": 9226,
    "tieba": 9227,
    "zhihu": 9228,
}


class CrawlerManager:
    """Crawler process manager (per-platform instance)"""

    def __init__(self, platform: str = ""):
        self._lock = asyncio.Lock()
        self.platform = platform
        self.platform_label = PLATFORM_NAMES.get(platform, platform)
        self.cdp_port = _CDP_PORT_MAP.get(platform, 9222)
        self.process: Optional[subprocess.Popen] = None
        self.status = "idle"
        self.started_at: Optional[datetime] = None
        self.current_config: Optional[CrawlerStartRequest] = None
        self._log_id = 0
        self._logs: List[LogEntry] = []
        self._read_task: Optional[asyncio.Task] = None
        self._project_root = Path(__file__).parent.parent.parent
        self._log_queue: Optional[asyncio.Queue] = None

        # Detect CDP_CONNECT_EXISTING from project config
        self._cdp_connect_existing = self._detect_cdp_connect_existing()

    def _detect_cdp_connect_existing(self) -> bool:
        """Check if config.CDP_CONNECT_EXISTING is True"""
        try:
            config_path = self._project_root / "config" / "base_config.py"
            if config_path.exists():
                text = config_path.read_text(encoding="utf-8")
                if "CDP_CONNECT_EXISTING = True" in text:
                    return True
        except Exception:
            pass
        return False

    @property
    def logs(self) -> List[LogEntry]:
        return self._logs

    def get_log_queue(self) -> asyncio.Queue:
        """Get or create log queue"""
        if self._log_queue is None:
            self._log_queue = asyncio.Queue()
        return self._log_queue

    def _create_log_entry(self, message: str, level: str = "info") -> LogEntry:
        """Create log entry with platform prefix"""
        self._log_id += 1
        if self.platform_label:
            message = f"[{self.platform_label}] {message}"
        entry = LogEntry(
            id=self._log_id,
            timestamp=datetime.now().strftime("%H:%M:%S"),
            level=level,
            message=message,
            platform=self.platform or None,
        )
        self._logs.append(entry)
        if len(self._logs) > 500:
            self._logs = self._logs[-500:]
        return entry

    async def _push_log(self, entry: LogEntry):
        """Push log to local queue and registry aggregated queue"""
        if self._log_queue is not None:
            try:
                self._log_queue.put_nowait(entry)
            except asyncio.QueueFull:
                pass

    def _parse_log_level(self, line: str) -> str:
        """Parse log level"""
        line_upper = line.upper()
        if "ERROR" in line_upper or "FAILED" in line_upper:
            return "error"
        elif "WARNING" in line_upper or "WARN" in line_upper:
            return "warning"
        elif "SUCCESS" in line_upper or "完成" in line or "成功" in line:
            return "success"
        elif "DEBUG" in line_upper:
            return "debug"
        return "info"

    async def start(self, config: CrawlerStartRequest) -> bool:
        """Start crawler process"""
        async with self._lock:
            if self.process and self.process.poll() is None:
                return False

            self._logs = []
            self._log_id = 0

            if self._log_queue is None:
                self._log_queue = asyncio.Queue()
            else:
                try:
                    while True:
                        self._log_queue.get_nowait()
                except asyncio.QueueEmpty:
                    pass

            cmd = self._build_command(config)

            entry = self._create_log_entry(f"启动爬虫: {' '.join(cmd)}", "info")
            await self._push_log(entry)

            try:
                self.process = subprocess.Popen(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.STDOUT,
                    text=True,
                    encoding='utf-8',
                    bufsize=1,
                    cwd=str(self._project_root),
                    env=self._build_env(config)
                )

                self.status = "running"
                self.started_at = datetime.now()
                self.current_config = config

                entry = self._create_log_entry(
                    f"爬虫已启动，类型: {config.crawler_type.value}",
                    "success"
                )
                await self._push_log(entry)

                self._read_task = asyncio.create_task(self._read_output())

                return True
            except Exception as e:
                self.status = "error"
                entry = self._create_log_entry(f"启动失败: {str(e)}", "error")
                await self._push_log(entry)
                return False

    async def stop(self) -> bool:
        """Stop crawler process"""
        async with self._lock:
            if not self.process or self.process.poll() is not None:
                return False

            self.status = "stopping"
            entry = self._create_log_entry("正在发送停止信号...", "warning")
            await self._push_log(entry)

            try:
                self.process.send_signal(signal.SIGTERM)

                for _ in range(30):
                    if self.process.poll() is not None:
                        break
                    await asyncio.sleep(0.5)

                if self.process.poll() is None:
                    entry = self._create_log_entry("进程未响应，强制终止...", "warning")
                    await self._push_log(entry)
                    self.process.kill()

                entry = self._create_log_entry("爬虫进程已终止", "info")
                await self._push_log(entry)

            except Exception as e:
                entry = self._create_log_entry(f"停止出错: {str(e)}", "error")
                await self._push_log(entry)

            self.status = "idle"
            self.current_config = None

            if self._read_task:
                self._read_task.cancel()
                self._read_task = None

            return True

    def get_status(self) -> dict:
        """Get current status"""
        return {
            "status": self.status,
            "platform": self.platform or (self.current_config.platform.value if self.current_config else None),
            "crawler_type": self.current_config.crawler_type.value if self.current_config else None,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "error_message": None,
        }

    def _build_env(self, config: CrawlerStartRequest) -> dict:
        """Build environment variables for proxy credentials"""
        env = {**os.environ, "PYTHONUNBUFFERED": "1"}
        if not config.enable_ip_proxy:
            return env
        provider = config.ip_proxy_provider.lower()
        if provider == "kuaidaili":
            if config.proxy_kdl_secret_id:
                env["KDL_SECERT_ID"] = config.proxy_kdl_secret_id
            if config.proxy_kdl_signature:
                env["KDL_SIGNATURE"] = config.proxy_kdl_signature
            if config.proxy_kdl_username:
                env["KDL_USER_NAME"] = config.proxy_kdl_username
            if config.proxy_kdl_password:
                env["KDL_USER_PWD"] = config.proxy_kdl_password
        elif provider == "wandouhttp":
            if config.proxy_wandou_app_key:
                env["WANDOU_APP_KEY"] = config.proxy_wandou_app_key
        return env

    def _build_command(self, config: CrawlerStartRequest) -> list:
        """Build main.py command line arguments"""
        cmd = ["uv", "run", "python", "main.py"]

        cmd.extend(["--platform", config.platform.value])
        cmd.extend(["--lt", config.login_type.value])
        cmd.extend(["--type", config.crawler_type.value])
        cmd.extend(["--save_data_option", config.save_option.value])

        if config.crawler_type.value == "search" and config.keywords:
            cmd.extend(["--keywords", config.keywords])
        elif config.crawler_type.value == "detail" and config.specified_ids:
            cmd.extend(["--specified_id", config.specified_ids])
        elif config.crawler_type.value == "creator" and config.creator_ids:
            cmd.extend(["--creator_id", config.creator_ids])

        if config.start_page != 1:
            cmd.extend(["--start", str(config.start_page)])

        cmd.extend(["--get_comment", "true" if config.enable_comments else "false"])
        cmd.extend(["--get_sub_comment", "true" if config.enable_sub_comments else "false"])

        if config.cookies:
            cmd.extend(["--cookies", config.cookies])

        cmd.extend(["--headless", "true" if config.headless else "false"])

        # Performance
        cmd.extend(["--max_notes_count", str(config.max_notes_count)])
        cmd.extend(["--max_comments_count_singlenotes", str(config.max_comments_count_singlenotes)])
        cmd.extend(["--max_sub_comments_count_singlenotes", str(config.max_sub_comments_count_singlenotes)])
        cmd.extend(["--max_concurrency_num", str(config.max_concurrency_num)])
        cmd.extend(["--sleep_sec_min", str(config.sleep_sec_min)])
        cmd.extend(["--sleep_sec_max", str(config.sleep_sec_max)])

        # CDP port: only isolate per-platform when launching own browser.
        # When CDP_CONNECT_EXISTING=True, all crawlers share the user's browser on default port.
        if not self._cdp_connect_existing:
            cmd.extend(["--cdp_port", str(self.cdp_port)])

        # Platform sort
        platform = config.platform.value
        if platform == "xhs":
            cmd.extend(["--xhs_sort_type", config.xhs_sort_type])
        elif platform == "wb":
            cmd.extend(["--weibo_search_type", config.weibo_search_type])
        elif platform == "dy":
            cmd.extend(["--dy_sort_type", str(config.dy_sort_type)])
        elif platform == "zhihu":
            cmd.extend(["--zhihu_sort", config.zhihu_sort])
            cmd.extend(["--zhihu_search_time", config.zhihu_search_time])

        # IP proxy
        cmd.extend(["--enable_ip_proxy", "true" if config.enable_ip_proxy else "false"])
        if config.enable_ip_proxy:
            cmd.extend(["--ip_proxy_pool_count", str(config.ip_proxy_pool_count)])
            cmd.extend(["--ip_proxy_provider_name", config.ip_proxy_provider])

        return cmd

    async def _read_output(self):
        """Asynchronously read process output"""
        loop = asyncio.get_event_loop()

        try:
            while self.process and self.process.poll() is None:
                line = await loop.run_in_executor(
                    None, self.process.stdout.readline
                )
                if line:
                    line = line.strip()
                    if line:
                        level = self._parse_log_level(line)
                        entry = self._create_log_entry(line, level)
                        await self._push_log(entry)

            if self.process and self.process.stdout:
                remaining = await loop.run_in_executor(
                    None, self.process.stdout.read
                )
                if remaining:
                    for line in remaining.strip().split('\n'):
                        if line.strip():
                            level = self._parse_log_level(line)
                            entry = self._create_log_entry(line.strip(), level)
                            await self._push_log(entry)

            if self.status == "running":
                exit_code = self.process.returncode if self.process else -1
                if exit_code == 0:
                    entry = self._create_log_entry("爬取完成", "success")
                else:
                    entry = self._create_log_entry(f"进程退出，代码: {exit_code}", "warning")
                await self._push_log(entry)
                self.status = "idle"

        except asyncio.CancelledError:
            pass
        except Exception as e:
            entry = self._create_log_entry(f"读取输出出错: {str(e)}", "error")
            await self._push_log(entry)


class CrawlerManagerRegistry:
    """Registry of per-platform CrawlerManager instances"""

    def __init__(self):
        self._managers: Dict[str, CrawlerManager] = {}
        self._agg_queue: asyncio.Queue = asyncio.Queue()

    def get(self, platform: str) -> CrawlerManager:
        """Get or create a manager for the given platform"""
        if platform not in self._managers:
            mgr = CrawlerManager(platform)
            # Patch _push_log to also push to aggregated queue
            original_push = mgr._push_log

            async def _push_both(entry: LogEntry, _orig=original_push):
                await _orig(entry)
                try:
                    self._agg_queue.put_nowait(entry)
                except asyncio.QueueFull:
                    pass

            mgr._push_log = _push_both
            self._managers[platform] = mgr
        return self._managers[platform]

    def get_all_status(self) -> List[dict]:
        """Get status of all registered managers"""
        return [mgr.get_status() for mgr in self._managers.values()]

    def get_running(self) -> List[CrawlerManager]:
        """Get all managers with running status"""
        return [mgr for mgr in self._managers.values() if mgr.status == "running"]

    async def stop_all(self) -> int:
        """Stop all running crawlers, return count of stopped"""
        count = 0
        for mgr in self._managers.values():
            if mgr.status == "running":
                if await mgr.stop():
                    count += 1
        return count

    def get_aggregated_queue(self) -> asyncio.Queue:
        """Get aggregated log queue (all platforms)"""
        return self._agg_queue

    def get_all_logs(self, limit: int = 200) -> List[LogEntry]:
        """Get recent logs from all managers, sorted by timestamp"""
        all_logs: List[LogEntry] = []
        for mgr in self._managers.values():
            all_logs.extend(mgr.logs)
        all_logs.sort(key=lambda x: x.id)
        return all_logs[-limit:] if limit > 0 else all_logs


# Global registry instance
crawler_registry = CrawlerManagerRegistry()

# Backward-compatible: default manager (no platform)
crawler_manager = CrawlerManager("")
