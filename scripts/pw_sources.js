// 公开替代源爬虫：百度百科 + 必应搜索学生内容文章
const { chromium } = require('playwright');
const fs = require('fs');

const OUT_DIR = '/root/tjut_docs';
const BAD_DOMAINS = ['xiaohongshu.com', 'zhihu.com', 'tieba.baidu.com', 'baidu.com/link', 'douyin.com', 'kuaishou.com'];

function clean(text) {
  const noise = /^(首页|登录|注册|搜索|导航|菜单|分享|收藏|评论|上一篇|下一篇|相关推荐|热门推荐|最新文章|点击查看更多|扫码关注|微信公众号|返回顶部|广告)/;
  return text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l.length > 6 && !noise.test(l))
    .filter((l, i, arr) => arr.indexOf(l) === i)
    .join('\n');
}

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    locale: 'zh-CN',
    viewport: { width: 1280, height: 900 },
  });
  const page = await ctx.newPage();

  // ===== 1. 百度百科词条 =====
  const baikeParts = ['# 07_百度百科词条（天津理工大学）\n'];
  try {
    await page.goto('https://baike.baidu.com/item/天津理工大学', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForTimeout(3000);
    const baike = await page.evaluate(() => {
      const main = document.querySelector('.J-lemma-content, .lemma-main-content');
      const basic = document.querySelector('.basic-info');
      return {
        text: main ? main.innerText : '',
        basic: basic ? basic.innerText : '',
      };
    });
    baikeParts.push('## 基本信息\n\n' + clean(baike.basic) + '\n');
    baikeParts.push('## 词条正文\n\n' + clean(baike.text) + '\n');
    console.log('✓ 百度百科:', baike.text.length, '字符');
  } catch (e) {
    console.log('✗ 百度百科:', e.message.slice(0, 60));
  }
  await page.waitForTimeout(800);

  // ===== 2. 必应搜索 + 打开文章 =====
  const queries = [
    '天津理工大学 食堂 怎么样',
    '天津理工大学 宿舍 条件',
    '天津理工大学 交通 地址 地铁',
    '天津理工大学 校园生活 新生',
  ];
  const results = []; // {url, title}
  for (const q of queries) {
    try {
      await page.goto('https://cn.bing.com/search?q=' + encodeURIComponent(q) + '&mkt=zh-CN', { waitUntil: 'domcontentloaded', timeout: 25000 });
      await page.waitForTimeout(2000);
      const links = await page.evaluate(() =>
        Array.from(document.querySelectorAll('li.b_algo h2 a, li.b_algo a.tilk')).map((a) => ({ t: a.innerText.trim(), h: a.href }))
      );
      for (const l of links) {
        if (l.h && !BAD_DOMAINS.some((d) => l.h.includes(d)) && !results.some((r) => r.url === l.h)) {
          results.push({ url: l.h, title: l.t });
        }
      }
      console.log(`搜索「${q}」→ 收集 ${links.length} 条`);
    } catch (e) {
      console.log(`搜索「${q}」失败: ${e.message.slice(0, 50)}`);
    }
    await page.waitForTimeout(1000);
  }

  const articleParts = ['# 08_校园生活（公开平台文章摘录）\n'];
  let okCount = 0;
  for (const r of results.slice(0, 14)) {
    if (okCount >= 8) break;
    try {
      await page.goto(r.url, { waitUntil: 'domcontentloaded', timeout: 25000 });
      await page.waitForTimeout(1800);
      const text = await page.evaluate(() => {
        const art = document.querySelector('article, .article, .content, #content, .post, .entry-content');
        return (art ? art.innerText : document.body.innerText).slice(0, 12000);
      });
      const cleaned = clean(text);
      if (cleaned.length < 300) {
        console.log('  跳过(内容太短):', r.title.slice(0, 30), r.url.slice(0, 50));
        continue;
      }
      articleParts.push(`\n## ${r.title}\n\n来源: ${r.url}\n\n${cleaned}\n`);
      okCount++;
      console.log('  ✓', r.title.slice(0, 40), `(${cleaned.length} 字符)`);
    } catch (e) {
      console.log('  ✗', r.url.slice(0, 60), e.message.slice(0, 40));
    }
    await page.waitForTimeout(1200);
  }

  fs.writeFileSync(`${OUT_DIR}/07_百度百科.md`, baikeParts.join('\n'));
  fs.writeFileSync(`${OUT_DIR}/08_校园生活.md`, articleParts.join('\n'));
  console.log(`\n入库文件: 07_百度百科(${baikeParts.join('').length} 字符), 08_校园生活(${articleParts.join('').length} 字符, ${okCount} 篇文章)`);
  await browser.close();
})();
