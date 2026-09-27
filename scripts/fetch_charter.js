const { chromium } = require('playwright');
const fs = require('fs');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const urls = [
    'http://zsb.tjut.edu.cn/info/1013/1686.htm', // 2026
    'http://zsb.tjut.edu.cn/info/1013/1617.htm', // 2025
  ];
  let out = '# 05_本科招生章程（天津理工大学）\n';
  for (const u of urls) {
    await page.goto(u, { waitUntil: 'networkidle', timeout: 30000 });
    await page.waitForTimeout(1200);
    const data = await page.evaluate(() => {
      const h = document.querySelector('h1, .title, .article-title');
      const body = document.body.innerText;
      return { title: h ? h.innerText.trim() : document.title, body };
    });
    // 清理页头页脚噪声
    const lines = data.body.split('\n')
      .map(l => l.trim())
      .filter(l => l && !/^(首页|学校概况|招生政策|招生章程|招生计划|专业介绍|网报入口|考生问答|历史资料|咨询热线|当前位置)/.test(l))
      .map(l => l.replace(/\s*点击量：\d+/, ''));
    out += `\n## ${data.title}\n\n来源: ${u}\n\n${lines.join('\n')}\n`;
    console.log(`✓ ${data.title} (${lines.join('\n').length} 字符)`);
  }
  fs.writeFileSync('/root/tjut_docs/05_本科招生.md', out);
  console.log('写入完成:', out.length, '字符');
  await browser.close();
})();
