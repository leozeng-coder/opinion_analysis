#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
DeepSentimentCrawling模块 - 平台爬虫管理器
负责配置和调用MediaCrawler进行多平台爬取
"""

import os
import sys
import subprocess
import tempfile
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Optional
import json

# 添加项目根目录到路径
project_root = Path(__file__).parent.parent
sys.path.append(str(project_root))

try:
    import config
except ImportError:
    raise ImportError("无法导入config.py配置文件")

class PlatformCrawler:
    """平台爬虫管理器"""
    
    def __init__(self):
        """初始化平台爬虫管理器"""
        self.mediacrawler_path = Path(__file__).parent / "MediaCrawler"
        self.supported_platforms = ['xhs', 'dy', 'ks', 'bili', 'wb', 'tieba', 'zhihu']
        self.crawl_stats = {}
        
        # 确保MediaCrawler目录存在
        if not self.mediacrawler_path.exists():
            raise FileNotFoundError(f"MediaCrawler目录不存在: {self.mediacrawler_path}")
        
        print(f"初始化平台爬虫管理器，MediaCrawler路径: {self.mediacrawler_path}")
    
    def configure_mediacrawler_db(self):
        """配置MediaCrawler使用我们的MySQL数据库"""
        try:
            # 修改MediaCrawler的数据库配置
            db_config_path = self.mediacrawler_path / "config" / "db_config.py"
            
            # 读取原始配置
            with open(db_config_path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # 替换数据库配置
            new_config = f'''# 声明：本代码仅供学习和研究目的使用。使用者应遵守以下原则：  
# 1. 不得用于任何商业用途。  
# 2. 使用时应遵守目标平台的使用条款和robots.txt规则。  
# 3. 不得进行大规模爬取或对平台造成运营干扰。  
# 4. 应合理控制请求频率，避免给目标平台带来不必要的负担。   
# 5. 不得用于任何非法或不当的用途。
#   
# 详细许可条款请参阅项目根目录下的LICENSE文件。  
# 使用本代码即表示您同意遵守上述原则和LICENSE中的所有条款。  


import os

# mysql config - 使用opinion_analysis数据库配置
MYSQL_DB_PWD = "{config.DB_PASSWORD}"
MYSQL_DB_USER = "{config.DB_USER}"
MYSQL_DB_HOST = "{config.DB_HOST}"
MYSQL_DB_PORT = {config.DB_PORT}
MYSQL_DB_NAME = "{config.DB_NAME}"


# redis config
REDIS_DB_HOST = "127.0.0.1"  # your redis host
REDIS_DB_PWD = os.getenv("REDIS_DB_PWD", "123456")  # your redis password
REDIS_DB_PORT = os.getenv("REDIS_DB_PORT", 6379)  # your redis port
REDIS_DB_NUM = os.getenv("REDIS_DB_NUM", 0)  # your redis db num

# cache type
CACHE_TYPE_REDIS = "redis"
CACHE_TYPE_MEMORY = "memory"

# sqlite config
SQLITE_DB_PATH = os.path.join(os.path.dirname(os.path.dirname(__file__)), "schema", "sqlite_tables.db")'''
            
            # 写入新配置
            with open(db_config_path, 'w', encoding='utf-8') as f:
                f.write(new_config)
            
            print("已配置MediaCrawler使用opinion_analysis数据库")
            return True
            
        except Exception as e:
            print(f"配置MediaCrawler数据库失败: {e}")
            return False
    
    def create_base_config(self, platform: str, keywords: List[str], 
                          crawler_type: str = "search", max_notes: int = 50) -> bool:
        """
        创建MediaCrawler的基础配置
        
        Args:
            platform: 平台名称
            keywords: 关键词列表
            crawler_type: 爬取类型
            max_notes: 最大爬取数量
        
        Returns:
            是否配置成功
        """
        try:
            base_config_path = self.mediacrawler_path / "config" / "base_config.py"
            
            # 将关键词列表转换为逗号分隔的字符串
            keywords_str = ",".join(keywords)
            
            # 读取原始配置文件
            with open(base_config_path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # 修改关键配置项
            lines = content.split('\n')
            new_lines = []
            
            for line in lines:
                if line.startswith('PLATFORM = '):
                    new_lines.append(f'PLATFORM = "{platform}"  # 平台，xhs | dy | ks | bili | wb | tieba | zhihu')
                elif line.startswith('KEYWORDS = '):
                    new_lines.append(f'KEYWORDS = "{keywords_str}"  # 关键词搜索配置，以英文逗号分隔')
                elif line.startswith('CRAWLER_TYPE = '):
                    new_lines.append(f'CRAWLER_TYPE = "{crawler_type}"  # 爬取类型，search(关键词搜索) | detail(帖子详情)| creator(创作者主页数据)')
                elif line.startswith('SAVE_DATA_OPTION = '):
                    new_lines.append('SAVE_DATA_OPTION = "db"  # csv or db or json or sqlite')
                elif line.startswith('CRAWLER_MAX_NOTES_COUNT = '):
                    new_lines.append(f'CRAWLER_MAX_NOTES_COUNT = {max_notes}')
                elif line.startswith('ENABLE_GET_COMMENTS = '):
                    new_lines.append('ENABLE_GET_COMMENTS = True')
                elif line.startswith('CRAWLER_MAX_COMMENTS_COUNT_SINGLENOTES = '):
                    new_lines.append('CRAWLER_MAX_COMMENTS_COUNT_SINGLENOTES = 20')
                elif line.startswith('HEADLESS = '):
                    new_lines.append('HEADLESS = True')  # 使用无头模式
                else:
                    new_lines.append(line)
            
            # 写入新配置
            with open(base_config_path, 'w', encoding='utf-8') as f:
                f.write('\n'.join(new_lines))
            
            print(f"已配置 {platform} 平台，关键词数量: {len(keywords)}")
            return True
            
        except Exception as e:
            print(f"创建基础配置失败: {e}")
            return False
    
    def run_crawler(self, platform: str, keywords: List[str], 
                   login_type: str = "qrcode", max_notes: int = 50) -> Dict:
        """
        运行爬虫
        
        Args:
            platform: 平台名称
            keywords: 关键词列表
            login_type: 登录方式
            max_notes: 最大爬取数量
        
        Returns:
            爬取结果统计
        """
        if platform not in self.supported_platforms:
            raise ValueError(f"不支持的平台: {platform}")
        
        if not keywords:
            raise ValueError("关键词列表不能为空")
        
        print(f"\n开始爬取平台: {platform}")
        print(f"关键词: {keywords[:5]}{'...' if len(keywords) > 5 else ''} (共{len(keywords)}个)")
        
        start_time = datetime.now()
        
        try:
            # 配置数据库
            if not self.configure_mediacrawler_db():
                return {"success": False, "error": "数据库配置失败"}
            
            # 创建基础配置
            if not self.create_base_config(platform, keywords, "search", max_notes):
                return {"success": False, "error": "基础配置创建失败"}
            
            # 构建命令
            cmd = [
                sys.executable, "main.py",
                "--platform", platform,
                "--lt", login_type,
                "--type", "search",
                "--save_data_option", "db"
            ]
            
            print(f"执行命令: {' '.join(cmd)}")
            
            # 切换到MediaCrawler目录并执行
            result = subprocess.run(
                cmd,
                cwd=self.mediacrawler_path,
                timeout=1800  # 30分钟超时
            )
            
            end_time = datetime.now()
            duration = (end_time - start_time).total_seconds()
            
            # 创建统计信息
            crawl_stats = {
                "platform": platform,
                "keywords_count": len(keywords),
                "duration_seconds": duration,
                "start_time": start_time.isoformat(),
                "end_time": end_time.isoformat(),
                "return_code": result.returncode,
                "success": result.returncode == 0,
                "notes_count": 0,
                "comments_count": 0,
                "errors_count": 0
            }
            
            # 保存统计信息
            self.crawl_stats[platform] = crawl_stats
            
            if result.returncode == 0:
                print(f"✅ {platform} 爬取完成，耗时: {duration:.1f}秒")
            else:
                print(f"❌ {platform} 爬取失败，返回码: {result.returncode}")
            
            return crawl_stats
            
        except subprocess.TimeoutExpired:
            print(f"❌ {platform} 爬取超时")
            return {"success": False, "error": "爬取超时", "platform": platform}
        except Exception as e:
            print(f"❌ {platform} 爬取异常: {e}")
            return {"success": False, "error": str(e), "platform": platform}
    
    def _parse_crawl_output(self, output_lines: List[str], error_lines: List[str]) -> Dict:
        """解析爬取输出，提取统计信息"""
        stats = {
            "notes_count": 0,
            "comments_count": 0,
            "errors_count": 0,
            "login_required": False
        }
        
        # 解析输出行
        for line in output_lines:
            if "条笔记" in line or "条内容" in line:
                try:
                    # 提取数字
                    import re
                    numbers = re.findall(r'\d+', line)
                    if numbers:
                        stats["notes_count"] = int(numbers[0])
                except:
                    pass
            elif "条评论" in line:
                try:
                    import re
                    numbers = re.findall(r'\d+', line)
                    if numbers:
                        stats["comments_count"] = int(numbers[0])
                except:
                    pass
            elif "登录" in line or "扫码" in line:
                stats["login_required"] = True
        
        # 解析错误行
        for line in error_lines:
            if "error" in line.lower() or "异常" in line:
                stats["errors_count"] += 1
        
        return stats
    
    def run_multi_platform_crawl_by_keywords(self, keywords: List[str], platforms: List[str],
                                            login_type: str = "qrcode", max_notes_per_keyword: int = 50) -> Dict:
        """
        基于关键词的多平台爬取 - 每个关键词在所有平台上都进行爬取
        
        Args:
            keywords: 关键词列表
            platforms: 平台列表
            login_type: 登录方式
            max_notes_per_keyword: 每个关键词在每个平台的最大爬取数量
        
        Returns:
            总体爬取统计
        """
        print(f"\n🚀 开始全平台关键词爬取")
        print(f"   关键词数量: {len(keywords)}")
        print(f"   平台数量: {len(platforms)}")
        print(f"   总爬取任务: {len(keywords)} × {len(platforms)} = {len(keywords) * len(platforms)}")
        
        total_stats = {
            "total_keywords": len(keywords),
            "total_platforms": len(platforms),
            "total_tasks": len(keywords) * len(platforms),
            "successful_tasks": 0,
            "failed_tasks": 0,
            "total_notes": 0,
            "total_comments": 0,
            "keyword_results": {},
            "platform_summary": {}
        }
        
        # 初始化平台统计
        for platform in platforms:
            total_stats["platform_summary"][platform] = {
                "successful_keywords": 0,
                "failed_keywords": 0,
                "total_notes": 0,
                "total_comments": 0
            }
        
        # 对每个平台一次性爬取所有关键词
        for platform in platforms:
            print(f"\n📝 在 {platform} 平台爬取所有关键词")
            print(f"   关键词: {', '.join(keywords[:5])}{'...' if len(keywords) > 5 else ''}")
            
            try:
                # 一次性传递所有关键词给平台
                result = self.run_crawler(platform, keywords, login_type, max_notes_per_keyword)
                
                if result.get("success"):
                    total_stats["successful_tasks"] += len(keywords)
                    total_stats["platform_summary"][platform]["successful_keywords"] = len(keywords)
                    
                    notes_count = result.get("notes_count", 0)
                    comments_count = result.get("comments_count", 0)
                    
                    total_stats["total_notes"] += notes_count
                    total_stats["total_comments"] += comments_count
                    total_stats["platform_summary"][platform]["total_notes"] = notes_count
                    total_stats["platform_summary"][platform]["total_comments"] = comments_count
                    
                    # 为每个关键词记录结果
                    for keyword in keywords:
                        if keyword not in total_stats["keyword_results"]:
                            total_stats["keyword_results"][keyword] = {}
                        total_stats["keyword_results"][keyword][platform] = result
                    
                    print(f"   ✅ 成功: {notes_count} 条内容, {comments_count} 条评论")
                else:
                    total_stats["failed_tasks"] += len(keywords)
                    total_stats["platform_summary"][platform]["failed_keywords"] = len(keywords)
                    
                    # 为每个关键词记录失败结果
                    for keyword in keywords:
                        if keyword not in total_stats["keyword_results"]:
                            total_stats["keyword_results"][keyword] = {}
                        total_stats["keyword_results"][keyword][platform] = result
                    
                    print(f"   ❌ 失败: {result.get('error', '未知错误')}")
            
            except Exception as e:
                total_stats["failed_tasks"] += len(keywords)
                total_stats["platform_summary"][platform]["failed_keywords"] = len(keywords)
                error_result = {"success": False, "error": str(e)}
                
                # 为每个关键词记录异常结果
                for keyword in keywords:
                    if keyword not in total_stats["keyword_results"]:
                        total_stats["keyword_results"][keyword] = {}
                    total_stats["keyword_results"][keyword][platform] = error_result
                
                print(f"   ❌ 异常: {e}")
        
        # 打印详细统计
        print(f"\n📊 全平台关键词爬取完成!")
        print(f"   总任务: {total_stats['total_tasks']}")
        print(f"   成功: {total_stats['successful_tasks']}")
        print(f"   失败: {total_stats['failed_tasks']}")
        print(f"   成功率: {total_stats['successful_tasks']/total_stats['total_tasks']*100:.1f}%")
        print(f"   总内容: {total_stats['total_notes']} 条")
        print(f"   总评论: {total_stats['total_comments']} 条")
        
        print(f"\n📈 各平台统计:")
        for platform, stats in total_stats["platform_summary"].items():
            success_rate = stats["successful_keywords"] / len(keywords) * 100 if keywords else 0
            print(f"   {platform}: {stats['successful_keywords']}/{len(keywords)} 关键词成功 ({success_rate:.1f}%), "
                  f"{stats['total_notes']} 条内容")
        
        return total_stats
    
    def get_crawl_statistics(self) -> Dict:
        """获取爬取统计信息"""
        return {
            "platforms_crawled": list(self.crawl_stats.keys()),
            "total_platforms": len(self.crawl_stats),
            "detailed_stats": self.crawl_stats
        }
    
    def save_crawl_log(self, log_path: str = None):
        """保存爬取日志"""
        if not log_path:
            log_path = f"crawl_log_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
        
        try:
            with open(log_path, 'w', encoding='utf-8') as f:
                json.dump(self.crawl_stats, f, ensure_ascii=False, indent=2)
            print(f"爬取日志已保存到: {log_path}")
        except Exception as e:
            print(f"保存爬取日志失败: {e}")

if __name__ == "__main__":
    # 测试平台爬虫管理器
    crawler = PlatformCrawler()
    
    # 测试配置
    test_keywords = ["科技", "AI", "编程"]
    result = crawler.run_crawler("xhs", test_keywords, max_notes=5)
    
    print(f"测试结果: {result}")
    print("平台爬虫管理器测试完成！")
