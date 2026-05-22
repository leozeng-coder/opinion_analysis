#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
数据查询示例脚本
演示如何从 MySQL 数据库中查询爬取的数据
"""
import asyncio
import sys
from pathlib import Path

# Add project root to sys.path
project_root = Path(__file__).resolve().parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

from database.db_session import get_session
from database.models import (
    XhsNote, XhsNoteComment, XhsCreator,
    DouyinAweme, DouyinAwemeComment,
    BilibiliVideo, BilibiliVideoComment,
    WeiboNote, WeiboNoteComment,
    KuaishouVideo, KuaishouVideoComment,
    TiebaNote, TiebaComment,
    ZhihuContent, ZhihuComment
)
from sqlalchemy import select, func, desc


async def query_xhs_notes(limit=10):
    """查询小红书笔记"""
    print("\n" + "=" * 80)
    print(f"📝 小红书笔记 (最新 {limit} 条)")
    print("=" * 80)

    async with get_session() as session:
        stmt = select(XhsNote).order_by(desc(XhsNote.add_ts)).limit(limit)
        result = await session.execute(stmt)
        notes = result.scalars().all()

        if not notes:
            print("暂无数据")
            return

        for i, note in enumerate(notes, 1):
            print(f"\n{i}. {note.title}")
            print(f"   作者: {note.nickname}")
            print(f"   点赞: {note.liked_count} | 收藏: {note.collected_count} | 评论: {note.comment_count}")
            print(f"   关键词: {note.source_keyword}")
            print(f"   链接: {note.note_url}")


async def query_xhs_comments(limit=10):
    """查询小红书评论"""
    print("\n" + "=" * 80)
    print(f"💬 小红书评论 (最新 {limit} 条)")
    print("=" * 80)

    async with get_session() as session:
        stmt = select(XhsNoteComment).order_by(desc(XhsNoteComment.add_ts)).limit(limit)
        result = await session.execute(stmt)
        comments = result.scalars().all()

        if not comments:
            print("暂无数据")
            return

        for i, comment in enumerate(comments, 1):
            content = comment.content[:50] + "..." if len(comment.content) > 50 else comment.content
            print(f"\n{i}. {comment.nickname}: {content}")
            print(f"   点赞: {comment.like_count} | 子评论: {comment.sub_comment_count}")
            print(f"   笔记ID: {comment.note_id}")


async def query_statistics():
    """查询统计信息"""
    print("\n" + "=" * 80)
    print("📊 数据统计")
    print("=" * 80)

    async with get_session() as session:
        # 统计各平台数据量
        stats = []

        # 小红书
        xhs_note_count = await session.scalar(select(func.count()).select_from(XhsNote))
        xhs_comment_count = await session.scalar(select(func.count()).select_from(XhsNoteComment))
        stats.append(("小红书笔记", xhs_note_count))
        stats.append(("小红书评论", xhs_comment_count))

        # 抖音
        dy_count = await session.scalar(select(func.count()).select_from(DouyinAweme))
        dy_comment_count = await session.scalar(select(func.count()).select_from(DouyinAwemeComment))
        stats.append(("抖音视频", dy_count))
        stats.append(("抖音评论", dy_comment_count))

        # B站
        bili_count = await session.scalar(select(func.count()).select_from(BilibiliVideo))
        bili_comment_count = await session.scalar(select(func.count()).select_from(BilibiliVideoComment))
        stats.append(("B站视频", bili_count))
        stats.append(("B站评论", bili_comment_count))

        # 微博
        wb_count = await session.scalar(select(func.count()).select_from(WeiboNote))
        wb_comment_count = await session.scalar(select(func.count()).select_from(WeiboNoteComment))
        stats.append(("微博笔记", wb_count))
        stats.append(("微博评论", wb_comment_count))

        # 快手
        ks_count = await session.scalar(select(func.count()).select_from(KuaishouVideo))
        ks_comment_count = await session.scalar(select(func.count()).select_from(KuaishouVideoComment))
        stats.append(("快手视频", ks_count))
        stats.append(("快手评论", ks_comment_count))

        # 贴吧
        tieba_count = await session.scalar(select(func.count()).select_from(TiebaNote))
        tieba_comment_count = await session.scalar(select(func.count()).select_from(TiebaComment))
        stats.append(("贴吧帖子", tieba_count))
        stats.append(("贴吧评论", tieba_comment_count))

        # 知乎
        zhihu_count = await session.scalar(select(func.count()).select_from(ZhihuContent))
        zhihu_comment_count = await session.scalar(select(func.count()).select_from(ZhihuComment))
        stats.append(("知乎内容", zhihu_count))
        stats.append(("知乎评论", zhihu_comment_count))

        print(f"\n{'平台':<15} {'数量':>10}")
        print("-" * 30)
        for name, count in stats:
            if count > 0:
                print(f"{name:<15} {count:>10,}")

        total = sum(count for _, count in stats)
        print("-" * 30)
        print(f"{'总计':<15} {total:>10,}")


async def query_top_notes_by_likes(platform="xhs", limit=5):
    """查询点赞数最高的笔记"""
    print("\n" + "=" * 80)
    print(f"🔥 {platform.upper()} 点赞数最高的内容 (Top {limit})")
    print("=" * 80)

    async with get_session() as session:
        if platform == "xhs":
            # 小红书需要将 Text 类型的 liked_count 转换为数字排序
            stmt = select(XhsNote).order_by(desc(XhsNote.liked_count)).limit(limit * 2)
            result = await session.execute(stmt)
            notes = result.scalars().all()

            # 手动排序（因为 liked_count 是 Text 类型）
            notes_sorted = sorted(notes, key=lambda x: int(x.liked_count) if x.liked_count.isdigit() else 0, reverse=True)[:limit]

            for i, note in enumerate(notes_sorted, 1):
                print(f"\n{i}. {note.title}")
                print(f"   作者: {note.nickname}")
                print(f"   点赞: {note.liked_count} | 收藏: {note.collected_count} | 评论: {note.comment_count}")
                print(f"   链接: {note.note_url}")


async def query_by_keyword(keyword, limit=10):
    """根据关键词查询"""
    print("\n" + "=" * 80)
    print(f"🔍 关键词搜索: '{keyword}' (最多 {limit} 条)")
    print("=" * 80)

    async with get_session() as session:
        stmt = select(XhsNote).where(
            XhsNote.source_keyword.like(f"%{keyword}%")
        ).order_by(desc(XhsNote.add_ts)).limit(limit)
        result = await session.execute(stmt)
        notes = result.scalars().all()

        if not notes:
            print(f"未找到包含关键词 '{keyword}' 的笔记")
            return

        print(f"\n找到 {len(notes)} 条相关笔记:\n")
        for i, note in enumerate(notes, 1):
            print(f"{i}. {note.title}")
            print(f"   作者: {note.nickname}")
            print(f"   点赞: {note.liked_count} | 评论: {note.comment_count}")
            print(f"   关键词: {note.source_keyword}")


async def main():
    """主函数"""
    print("\n" + "=" * 80)
    print("🚀 MediaCrawler 数据查询工具")
    print("=" * 80)

    try:
        # 1. 统计信息
        await query_statistics()

        # 2. 查询小红书笔记
        await query_xhs_notes(limit=5)

        # 3. 查询小红书评论
        await query_xhs_comments(limit=5)

        # 4. 查询点赞最高的笔记
        await query_top_notes_by_likes(platform="xhs", limit=5)

        # 5. 关键词搜索示例（如果有数据）
        # await query_by_keyword("编程", limit=5)

        print("\n" + "=" * 80)
        print("✓ 查询完成")
        print("=" * 80 + "\n")

    except Exception as e:
        print(f"\n✗ 查询失败: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    asyncio.run(main())
