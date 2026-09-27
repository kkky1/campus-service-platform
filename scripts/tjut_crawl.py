#!/usr/bin/env python3
"""天津理工大学官网爬虫 → Markdown 文档（用于 RAG 知识库）。

- 只抓公开页面，限速 0.6s/请求
- 按主题聚合为若干 .md 文件，输出到 /root/tjut_docs/
"""
import os
import re
import time
import urllib.error
import urllib.request
from html.parser import HTMLParser

BASE = "https://www.tjut.edu.cn"
OUT = "/root/tjut_docs"
UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
DELAY = 0.6

os.makedirs(OUT, exist_ok=True)


class TextExtractor(HTMLParser):
    """把 HTML 提取为带标题层级的纯文本行。"""

    SKIP = {"script", "style", "nav", "footer", "noscript", "iframe", "form"}

    def __init__(self):
        super().__init__()
        self.lines = []
        self._skip_depth = 0
        self._cur = []
        self._heading = 0
        self._in_title = False
        self.title = ""

    def handle_starttag(self, tag, attrs):
        if tag in self.SKIP:
            self._skip_depth += 1
        if tag in ("h1", "h2", "h3", "h4"):
            self.flush()
            self._heading = int(tag[1])
        if tag == "title":
            self._in_title = True
        if tag in ("p", "div", "li", "br", "tr", "td"):
            self.flush()

    def handle_endtag(self, tag):
        if tag in self.SKIP and self._skip_depth > 0:
            self._skip_depth -= 1
        if tag == "title":
            self._in_title = False
        if tag in ("h1", "h2", "h3", "h4"):
            self._heading = 0
            self.flush()
        if tag in ("p", "div", "li", "tr"):
            self.flush()

    def handle_data(self, data):
        if self._skip_depth:
            return
        text = data.strip()
        if not text:
            return
        if self._in_title:
            self.title += text
        self._cur.append(text)

    def flush(self):
        text = " ".join(self._cur).strip()
        self._cur = []
        if not text or len(text) < 2:
            return
        if self._heading:
            self.lines.append("#" * self._heading + " " + text)
        else:
            self.lines.append(text)


def fetch(url, timeout=20):
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept-Language": "zh-CN,zh;q=0.9"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        raw = r.read()
    for enc in ("utf-8", "gbk"):
        try:
            return raw.decode(enc)
        except UnicodeDecodeError:
            continue
    return raw.decode("utf-8", errors="ignore")


def page_to_md(url):
    """返回 (title, markdown_text)。"""
    html = fetch(url)
    p = TextExtractor()
    p.feed(html)
    title = (p.title or "").strip()
    # 过滤导航噪声行
    noise = re.compile(r"^(首页|学校简介|现任领导|历任领导|English|返回|上一条|下一条|上一篇|下一篇|分享到|打印|关闭|更多|查看更多)$")
    lines = [l for l in p.lines if not noise.match(l.strip())]
    # 去掉连续重复行
    dedup = []
    for l in lines:
        if not dedup or dedup[-1] != l:
            dedup.append(l)
    return title, "\n\n".join(dedup)


def extract_links(html, pattern):
    links = []
    for href, text in re.findall(r'<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>', html, re.S):
        text = re.sub(r"<[^>]+>", "", text).strip()
        if pattern.search(href) and text:
            if href.startswith("http"):
                links.append((href, text))
            else:
                links.append((BASE + "/" + href.lstrip("./"), text))
    # 去重保序
    seen, out = set(), []
    for u, t in links:
        if u in seen:
            continue
        seen.add(u)
        out.append((u, t))
    return out


def crawl_section(name, urls, max_pages=20):
    """抓取一组页面，凑成一个主题 md。"""
    parts = [f"# {name}（天津理工大学官网）\n"]
    for url in urls[:max_pages]:
        try:
            title, text = page_to_md(url)
            print(f"  ✓ {title or url}  {url}")
            parts.append(f"\n## {title or name}\n\n来源: {url}\n\n{text}\n")
        except Exception as e:
            print(f"  ✗ 失败 {url}: {e}")
        time.sleep(DELAY)
    path = os.path.join(OUT, name + ".md")
    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(parts))
    print(f"  → 写入 {path}（{len(''.join(parts))} 字符）")
    return path


def main():
    # 1) 概况类固定页面
    crawl_section("01_学校概况", [
        f"{BASE}/index.htm",
        f"{BASE}/tlgk/xxjj.htm",
        f"{BASE}/tlgk/xxlc.htm",
        f"{BASE}/tlgk/xrld.htm",
        f"{BASE}/tlgk/zhgljgjqtzz.htm",
        f"{BASE}/tlgk/jxkyjg.htm",
        f"{BASE}/rcpy.htm",
    ])

    # 2) 新闻/通知列表 → 抓最新 N 条
    for name, list_url, pat, n in [
        ("02_理工要闻", f"{BASE}/zhxw/lgyw.htm", re.compile(r"info/\d+/\d+\.htm"), 12),
        ("03_通知公告", f"{BASE}/zhxw/tzgg.htm", re.compile(r"info/\d+/\d+\.htm"), 12),
    ]:
        try:
            html = fetch(list_url)
            links = extract_links(html, pat)
            print(f"{name}: 列表页发现 {len(links)} 条")
            crawl_section(name, [u for u, _ in links], max_pages=n)
        except Exception as e:
            print(f"{name} 列表抓取失败: {e}")

    # 3) 图书馆（独立站点）
    try:
        html = fetch("https://lib.tjut.edu.cn/")
        links = extract_links(html, re.compile(r"info/\d+/\d+\.htm|\.htm"))
        # 挑与开放/服务相关的
        picked = [u for u, t in links if any(k in t for k in ("开放", "时间", "服务", "借阅", "入馆", "概况", "简介"))][:6]
        if not picked:
            picked = [u for u, _ in links[:6]]
        lib_pages = ["https://lib.tjut.edu.cn/"] + picked
        crawl_section("04_图书馆", lib_pages, max_pages=8)
    except Exception as e:
        print(f"图书馆抓取失败: {e}")

    # 4) 本科招生网
    try:
        html = fetch("http://zsb.tjut.edu.cn/")
        links = extract_links(html, re.compile(r"info/\d+/\d+\.htm|zsxx|bkzn|.htm"))
        picked = [u for u, t in links if any(k in t for k in ("招生", "章程", "简章", "计划", "专业", "录取", "问答", "指南"))][:8]
        crawl_section("05_本科招生", ["http://zsb.tjut.edu.cn/"] + picked, max_pages=10)
    except Exception as e:
        print(f"招生网抓取失败: {e}")


if __name__ == "__main__":
    main()
