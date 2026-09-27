#!/usr/bin/env python3
"""第二轮：定向补充图书馆/招生/生活服务内容。"""
import os, re, time, urllib.parse, urllib.request
from html.parser import HTMLParser
import sys
sys.path.insert(0, '/tmp')
from tjut_crawl import fetch, page_to_md, extract_links, TextExtractor, BASE, OUT, DELAY

def links_of(url):
    html = fetch(url)
    out = []
    for href, text in re.findall(r'<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>', html, re.S):
        text = re.sub(r'<[^>]+>', '', text).strip()
        if not href or href.startswith('#') or href.startswith('javascript'):
            continue
        full = urllib.parse.urljoin(url, href)
        if text:
            out.append((full, text))
    seen, res = set(), []
    for u, t in out:
        if u in seen: continue
        seen.add(u); res.append((u, t))
    return res

def crawl(name, urls, max_pages=15):
    parts = [f"# {name}（天津理工大学）\n"]
    for url in urls[:max_pages]:
        try:
            title, text = page_to_md(url)
            print(f"  ✓ {title or url}")
            parts.append(f"\n## {title or name}\n\n来源: {url}\n\n{text}\n")
        except Exception as e:
            print(f"  ✗ {url}: {str(e)[:60]}")
        time.sleep(DELAY)
    path = os.path.join(OUT, name + ".md")
    open(path, "w", encoding="utf-8").write("\n".join(parts))
    print(f"  → {path} ({len(''.join(parts))} 字符)")

# 图书馆：找开放时间/入馆/借阅相关
print("【图书馆】")
lib_links = links_of("https://lib.tjut.edu.cn/")
key = [u for u, t in lib_links if any(k in t for k in ("开放时间", "开馆", "入馆", "借阅", "规则", "概况", "简介", "服务", "座位", "预约"))][:8]
crawl("04_图书馆", ["https://lib.tjut.edu.cn/"] + key, max_pages=9)

# 招生：章程/计划/问答
print("【本科招生】")
zs_links = links_of("http://zsb.tjut.edu.cn/")
key = [u for u, t in zs_links if any(k in t for k in ("章程", "简章", "计划", "专业", "录取", "问答", "指南", "招生", "分数"))][:8]
crawl("05_本科招生", ["http://zsb.tjut.edu.cn/"] + key, max_pages=9)

# 生活服务：探测后勤/迎新站点
print("【生活服务探测】")
candidates = [
    "https://hqc.tjut.edu.cn/",        # 后勤管理处
    "https://yx.tjut.edu.cn/",         # 迎新网
    "https://xyh.tjut.edu.cn/",        # 校友
    f"{BASE}/tlgk/lxfs.htm",           # 联系方式
]
alive = []
for u in candidates:
    try:
        html = fetch(u, timeout=10)
        print(f"  ✓ 可达: {u} ({len(html)} 字符)")
        alive.append(u)
    except Exception as e:
        print(f"  ✗ 不可达: {u} ({str(e)[:40]})")
if alive:
    crawl("06_生活服务与联系方式", alive, max_pages=6)
