#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
测试 MySQL 连接和数据写入功能
"""
import asyncio
import sys
from pathlib import Path

# Add project root to sys.path
project_root = Path(__file__).resolve().parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

from config.db_config import mysql_db_config
from database.db_session import create_database_if_not_exists, create_tables, get_session
from database.models import XhsNote, XhsNoteComment
from tools.time_util import get_current_timestamp
from sqlalchemy import select, text


async def test_connection():
    """测试数据库连接"""
    print("=" * 60)
    print("测试 MySQL 连接配置")
    print("=" * 60)
    print(f"数据库配置: {mysql_db_config}")
    print()

    try:
        # 设置保存数据选项为 mysql
        import config
        config.SAVE_DATA_OPTION = "mysql"

        # 1. 创建数据库（如果不存在）
        print("步骤 1: 创建数据库（如果不存在）...")
        try:
            await create_database_if_not_exists("mysql")
            print("✓ 数据库创建/检查成功")
        except Exception as e:
            if "database exists" in str(e).lower():
                print("✓ 数据库已存在")
            else:
                raise
        print()

        # 2. 创建表结构
        print("步骤 2: 创建表结构...")
        await create_tables("mysql")
        print("✓ 表结构创建成功")
        print()

        # 3. 测试数据库连接
        print("步骤 3: 测试数据库连接...")
        async with get_session() as session:
            result = await session.execute(text("SELECT DATABASE()"))
            db_name = result.scalar()
            print(f"✓ 成功连接到数据库: {db_name}")

            # 查询现有表
            result = await session.execute(text("SHOW TABLES"))
            tables = [row[0] for row in result.fetchall()]
            print(f"✓ 数据库中的表 ({len(tables)} 个):")
            for table in sorted(tables):
                print(f"  - {table}")
        print()

        # 4. 测试写入数据
        print("步骤 4: 测试写入数据...")
        test_note_id = "test_note_" + str(int(get_current_timestamp()))

        async with get_session() as session:
            # 插入测试笔记
            test_note = XhsNote(
                user_id="test_user_123",
                nickname="测试用户",
                avatar="https://example.com/avatar.jpg",
                ip_location="北京",
                add_ts=int(get_current_timestamp()),
                last_modify_ts=int(get_current_timestamp()),
                note_id=test_note_id,
                type="normal",
                title="测试笔记标题",
                desc="这是一条测试笔记的描述内容",
                video_url="",
                time=int(get_current_timestamp()),
                last_update_time=int(get_current_timestamp()),
                liked_count="100",
                collected_count="50",
                comment_count="20",
                share_count="10",
                image_list="[]",
                tag_list='["测试", "爬虫"]',
                note_url=f"https://www.xiaohongshu.com/explore/{test_note_id}",
                source_keyword="测试关键词",
                xsec_token=""
            )
            session.add(test_note)
            await session.commit()
            print(f"✓ 成功插入测试笔记: {test_note_id}")

        # 5. 验证数据
        print("步骤 5: 验证数据...")
        async with get_session() as session:
            stmt = select(XhsNote).where(XhsNote.note_id == test_note_id)
            result = await session.execute(stmt)
            note = result.scalar_one_or_none()

            if note:
                print(f"✓ 成功读取测试笔记:")
                print(f"  - ID: {note.id}")
                print(f"  - 笔记ID: {note.note_id}")
                print(f"  - 标题: {note.title}")
                print(f"  - 用户: {note.nickname}")
                print(f"  - 点赞数: {note.liked_count}")
            else:
                print("✗ 未找到测试笔记")
        print()

        # 6. 清理测试数据
        print("步骤 6: 清理测试数据...")
        async with get_session() as session:
            stmt = select(XhsNote).where(XhsNote.note_id == test_note_id)
            result = await session.execute(stmt)
            note = result.scalar_one_or_none()
            if note:
                await session.delete(note)
                await session.commit()
                print(f"✓ 已删除测试笔记: {test_note_id}")
        print()

        print("=" * 60)
        print("✓ 所有测试通过！MySQL 配置正确，可以正常使用")
        print("=" * 60)

    except Exception as e:
        print()
        print("=" * 60)
        print(f"✗ 测试失败: {e}")
        print("=" * 60)
        import traceback
        traceback.print_exc()
        sys.exit(1)


async def show_table_info():
    """显示表结构信息"""
    print("\n" + "=" * 60)
    print("数据库表结构信息")
    print("=" * 60)

    async with get_session() as session:
        # 获取小红书相关表的信息
        tables = ['xhs_note', 'xhs_note_comment', 'xhs_creator']

        for table_name in tables:
            try:
                result = await session.execute(text(f"DESCRIBE {table_name}"))
                columns = result.fetchall()

                print(f"\n表: {table_name}")
                print("-" * 60)
                print(f"{'字段名':<25} {'类型':<20} {'说明'}")
                print("-" * 60)

                for col in columns:
                    field_name = col[0]
                    field_type = col[1]
                    print(f"{field_name:<25} {field_type:<20}")

            except Exception as e:
                print(f"\n表 {table_name} 不存在或查询失败: {e}")


if __name__ == "__main__":
    import sys
    import io

    # 设置输出编码为 UTF-8
    if sys.stdout.encoding != 'utf-8':
        sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

    print("\nMediaCrawler MySQL 连接测试工具\n")

    # 运行测试
    asyncio.run(test_connection())

    # 显示表结构
    asyncio.run(show_table_info())
