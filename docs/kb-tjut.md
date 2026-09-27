# 天津理工大学校园知识库（RAG 数据）

## 当前状态

- 知识库：**天津理工大学校园知识库**（RAG `kbId=10`）
- 语料：**9 篇文档、460 个切片**（官网 + 百科 + 公开平台 + 后勤官方）
- 语料文件：`docs/kb/tjut/`（可重复上传重建）

| 文档 | 内容 | 来源 |
|---|---|---|
| 01_学校概况.md | 学校简介、兴学历程、现任领导、机构设置、教学科研机构、人才培养 | www.tjut.edu.cn |
| 02_理工要闻.md | 近期学校新闻（12 条） | zhxw/lgyw |
| 03_通知公告.md | 近期通知公告（12 条） | zhxw/tzgg |
| 04_图书馆.md | 图书馆概况/服务简介/规章制度/学科服务 | lib.tjut.edu.cn |
| 05_本科招生.md | 2026/2025 年普通本科招生章程全文 | zsb.tjut.edu.cn |
| 06_生活服务与联系方式.md | 后勤管理处、饮食/物业/联系我们 | hqc.tjut.edu.cn |
| 07_百度百科.md | 百科词条全文（历史/学科/校园等） | 百度百科（无头浏览器渲染） |
| 08_校园生活.md | 宿舍/交通/校园卡/新生攻略等 10 篇公开文章 | 必应公开结果（大学生必备网等） |
| 09_后勤服务.md | 机构设置、物业迎新、校园美食节、食品安全 | hqc.tjut.edu.cn |

## 如何重建

```bash
python3 scripts/tjut_crawl.py          # 官网主站：概况/要闻/通知/图书馆/招生
python3 scripts/tjut_crawl2.py         # 定向补充：图书馆细节/招生章程/后勤
node scripts/fetch_charter.js          # 招生章程正文（JS 渲染页，用 Playwright）
# 然后通过 API 创建知识库并上传 docs/kb/tjut/*.md（rag.html 页面也可手动上传）
```

## 抓取工具链

| 脚本 | 用途 |
|---|---|
| `scripts/tjut_crawl.py` / `tjut_crawl2.py` | 官网主站/图书馆/招生/后勤（HTTP 直连） |
| `scripts/fetch_charter.js` | 招生章程正文（JS 渲染页，Playwright） |
| `scripts/pw_sources.js` / `pw_sources2.js` | 百度百科词条 + 必应定向搜索公开文章（Playwright 过反爬） |

## 小红书数据说明

小红书网页端**必须登录**（搜索/笔记数据不在初始 HTML 中，且有反爬与数据中心 IP 风控），
无法在无人值守的情况下抓取。可选方案见对话说明（提供登录 Cookie / 手动导出内容 / 换公开来源）。
