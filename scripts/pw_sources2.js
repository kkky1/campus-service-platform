// v2：后勤官方服务页 + 定向学生内容搜索（标题/域名过滤）
const { chromium } = require('playwright');
const fs = require('fs');

const OUT_DIR = '/root/tjut_docs';
const BAD = ['xiaohongshu.com', 'zhihu.com', 'tieba.baidu.com', 'baidu.com/link', 'douyin.com', 'kuaishou.com', 'tripadvisor', 'baike.baidu.com/item/天津市'];
const GOOD_DOMAINS = ['dxsbb.com', 'bendibao.com', 'gaokao', 'eol.cn', 'worlduc.com', 'sushe.help', 'zhaosheng.com', 'xuexila.com', 'daxuecn.com', 'gaosan.com', 'gkw.com.cn', '027art.com', 'eduei.com', 'cnki', 'gaokao.cn', 'gk100.com', 'youzy.cn'];

function clean(text) {
  const noise = /^(首页|登录|注册|搜索|导航|菜单|分享|收藏|评论|上一篇|下一篇|相关推荐|热门推荐|最新文章|点击查看更多|扫码关注|微信公众号|返回顶部|广告|当前位置)/;
  return text.split('\n').map((l) => l.trim()).filter((l) => l.length > 8 && !noise.test(l)).filter((l, i, a) => a.indexOf(l) === i).join('\n');
}

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    locale: 'zh-CN', viewport: { width: 1280, height: 900 },
  });
  const page = await ctx.newPage();

  // ===== A. 后勤管理处：机构与服务页 =====
  const hqcParts = ['# 09_后勤服务（天津理工大学后勤管理处）\n'];
  try {
    await page.goto('https://hqc.tjut.edu.cn/', { waitUntil: 'domcontentloaded', timeout: 25000 });
    await page.waitForTimeout(2000);
    const links = await page.evaluate(() =>
      Array.from(document.querySelectorAll('a')).map((a) => ({ t: a.innerText.trim(), h: a.href })).filter((x) => x.t && /机构|服务|饮食|物业|维修|公寓|能源|校园|联系|指南|介绍/.test(x.t))
    );
    const picked = [];
    for (const l of links) {
      if (picked.some((p) => p.h === l.h)) continue;
      picked.push(l);
      if (picked.length >= 8) break;
    }
    console.log('后勤相关页:', picked.map((p) => p.t).join(', '));
    for (const p of picked) {
      try {
        await page.goto(p.h, { waitUntil: 'domcontentloaded', timeout: 20000 });
        await page.waitForTimeout(1200);
        const data = await page.evaluate(() => ({ title: document.title, text: document.body.innerText }));
        const c = clean(data.text);
        if (c.length > 200) {
          hqcParts.push(`\n## ${p.t}（${data.title.split('-')[0]}）\n\n来源: ${p.h}\n\n${c.slice(0, 6000)}\n`);
          console.log('  ✓', p.t, c.length);
        }
      } catch (e) { console.log('  ✗', p.t, e.message.slice(0, 30)); }
      await page.waitForTimeout(900);
    }
  } catch (e) { console.log('后勤失败:', e.message.slice(0, 60)); }
  fs.writeFileSync(`${OUT_DIR}/09_后勤服务.md`, hqcParts.join('\n'));
  console.log('写入 09_后勤服务.md:', hqcParts.join('').length, '字符');

  // ===== B. 定向搜索学生内容 =====
  const queries = [
    '天津理工大学 食堂 好不好 排名',
    '天津理工大学 宿舍 几人间 空调 独卫',
    '天津理工大学 快递 超市 校园卡',
    '天津理工大学 怎么走 地铁 公交 宾水西道',
    '天津理工大学 新生 报到 攻略',
    '天津理工大学 图书馆 自习 座位',
  ];
  const results = [];
  for (const q of queries) {
    try {
      await page.goto('https://cn.bing.com/search?q=' + encodeURIComponent(q) + '&mkt=zh-CN', { waitUntil: 'domcontentloaded', timeout: 25000 });
      await page.waitForTimeout(1800);
      const links = await page.evaluate(() =>
        Array.from(document.querySelectorAll('li.b_algo h2 a')).map((a) => ({ t: a.innerText.trim(), h: a.href }))
      );
      for (const l of links) {
        if (!l.h || BAD.some((d) => l.h.includes(d))) continue;
        const relevant = l.t.includes('天津理工') || l.h.includes('tianjinligong') || GOOD_DOMAINS.some((d) => l.h.includes(d)) || /宿舍|食堂|校园|新生|攻略|交通/.test(l.t);
        if (relevant && !results.some((r) => r.h === l.h)) results.push(l);
      }
      console.log(`「${q}」→ 累计候选 ${results.length}`);
    } catch (e) { console.log(`搜索失败: ${q} ${e.message.slice(0, 40)}`); }
    await page.waitForTimeout(1000);
  }

  const artParts = ['# 08_校园生活（公开平台：宿舍/食堂/交通/新生攻略）\n'];
  let ok = 0;
  for (const r of results) {
    if (ok >= 10) break;
    try {
      await page.goto(r.h, { waitUntil: 'domcontentloaded', timeout: 22000 });
      await page.waitForTimeout(1500);
      const data = await page.evaluate(() => {
        const art = document.querySelector('article, .article, .content, #content, .post, .entry-content, .article-content');
        return { title: document.title, text: (art ? art.innerText : document.body.innerText).slice(0, 12000) };
      });
      const c = clean(data.text);
      if (c.length < 400) continue;
      artParts.push(`\n## ${r.t}\n\n来源: ${r.h}\n\n${c}\n`);
      ok++;
      console.log('  ✓', r.t.slice(0, 50), `(${c.length})`);
    } catch (e) { /* skip */ }
    await page.waitForTimeout(1100);
  }
  fs.writeFileSync(`${OUT_DIR}/08_校园生活.md`, artParts.join('\n'));
  console.log(`写入 08_校园生活.md: ${artParts.join('').length} 字符, ${ok} 篇`);
  await browser.close();
})();
