import asyncio
import aiomysql
from async_db import AsyncMysqlDB
from config.db_config import *

async def test():
    # 创建连接池
    pool = await aiomysql.create_pool(
        host=MYSQL_DB_HOST,
        port=MYSQL_DB_PORT,
        user=MYSQL_DB_USER,
        password=MYSQL_DB_PWD,
        db=MYSQL_DB_NAME,
        autocommit=True,
    )

    db = AsyncMysqlDB(pool)

    # 测试查询
    result = await db.query('SELECT COUNT(*) as cnt FROM tieba_note')
    print(f'tieba_note count: {result}')

    result2 = await db.query('SELECT COUNT(*) as cnt FROM tieba_comment')
    print(f'tieba_comment count: {result2}')

    # 测试插入
    test_data = {
        'note_id': 'test_123',
        'title': 'test title',
        'desc': 'test desc',
        'note_url': 'http://test.com',
        'publish_time': '2026-05-22 18:00:00',
        'user_nickname': 'test user',
        'tieba_name': 'test tieba',
        'tieba_link': 'http://test.com',
        'add_ts': 1779444444000,
        'last_modify_ts': 1779444444000
    }

    try:
        row_id = await db.item_to_table('tieba_note', test_data)
        print(f'Insert success, row_id: {row_id}')

        # 删除测试数据
        await db.execute('DELETE FROM tieba_note WHERE note_id = "test_123"')
        print('Test data cleaned')
    except Exception as e:
        print(f'Insert failed: {e}')
        import traceback
        traceback.print_exc()

    pool.close()
    await pool.wait_closed()

if __name__ == '__main__':
    asyncio.run(test())
