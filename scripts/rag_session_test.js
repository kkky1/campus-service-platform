const { chromium } = require('playwright');
(async () => {
  const BASE = 'http://127.0.0.1:8080';
  const login = await (await fetch(BASE + '/api/user/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ phone: '13800000001' }),
  })).json();
  const token = login.data;
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 420, height: 900 }, locale: 'zh-CN' });
  await ctx.addInitScript((t) => {
    sessionStorage.setItem('token', t);
    localStorage.setItem('ragKbId', '10'); // 天津理工大学校园知识库
  }, token);
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message.slice(0, 120)));

  const ask = async (q) => {
    await page.locator('.input-bar input').fill(q);
    await page.locator('.input-bar button').click();
    await page.locator('.msg .a').last().waitFor({ timeout: 90000 });
    await page.waitForTimeout(800);
  };

  await page.goto(BASE + '/rag.html', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // 确保选中天津知识库
  const kbText = await page.locator('.kb-select').innerText().catch(() => '');
  console.log('当前知识库:', kbText.replace(/\n/g, ' '));

  // 新建会话 → 问第一题
  await page.locator('button', { hasText: '新会话' }).click();
  await page.waitForTimeout(500);
  await ask('图书馆开放时间？');
  const sessAfterQ1 = await page.evaluate(() => document.getElementById('app').__vue__.sessionId);
  console.log('第一题后 sessionId:', sessAfterQ1);
  const msgCount1 = await page.locator('.msg').count();
  console.log('消息数:', msgCount1);

  // 再问一题（同会话）
  await ask('宿舍有空调吗？');
  const msgCount2 = await page.locator('.msg').count();
  console.log('两题后消息数:', msgCount2);

  // ==== 引用 bug 验证：点第一条回答的角标，应显示第一条消息对应的引用切片 ====
  const firstCite = page.locator('.msg .a').first().locator('.cite').first();
  const citeId = await firstCite.getAttribute('data-id');
  const firstMi = await page.locator('.msg .a').first().locator('.bubble > div').first().getAttribute('data-mi');
  await firstCite.click();
  await page.waitForTimeout(600);
  const panelText = await page.locator('.detail-panel').innerText();
  const expectText = await page.evaluate(([mi, cid]) => {
    const vm = document.getElementById('app').__vue__;
    const msg = vm.messages[Number(mi)];
    return (msg.refs[Number(cid)] || {}).text || '';
  }, [firstMi, citeId]);
  const lastRefText = await page.evaluate(() => {
    const vm = document.getElementById('app').__vue__;
    const answers = vm.messages.filter((m) => m.role === 'assistant');
    const last = answers[answers.length - 1];
    return (last.refs[0] || {}).text || '';
  });
  const matched = expectText && panelText.includes(expectText.slice(0, 24).replace(/\s+/g, ' ')) || (expectText && panelText.replace(/\s+/g,'').includes(expectText.slice(0, 24).replace(/\s+/g, '')));
  console.log(`引用修复验证: 点击 [ID:${citeId}] @消息${firstMi} → 面板匹配第一条消息引用:`, matched ? '✓' : '✗');
  console.log('  面板:', panelText.slice(0, 60).replace(/\n/g, ' '));
  console.log('  期望(第一条):', expectText.slice(0, 40), '| 最后一条的refs[0]:', lastRefText.slice(0, 30));

  // ==== 刷新后历史持久化 ====
  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForTimeout(2500);
  const afterReload = await page.locator('.msg').count();
  const sessAfterReload = await page.evaluate(() => document.getElementById('app').__vue__.sessionId);
  console.log('刷新后消息数:', afterReload, '| sessionId:', sessAfterReload, afterReload >= 4 ? '✓ 历史保留' : '✗ 历史丢失');

  // ==== 新会话隔离 ====
  await page.locator('button', { hasText: '新会话' }).click();
  await page.waitForTimeout(500);
  const freshCount = await page.locator('.msg').count();
  await ask('食堂在哪？');
  const newSess = await page.evaluate(() => document.getElementById('app').__vue__.sessionId);
  const newCount = await page.locator('.msg').count();
  console.log('新会话消息数(应2):', newCount, '| 新会话ID:', newSess, '| 与旧会话不同:', newSess !== sessAfterReload ? '✓' : '✗');

  // ==== 会话列表与删除 ====
  const sessOptions = await page.locator('.session-select').click().then(async () => {
    await page.waitForTimeout(500);
    const n = await page.locator('.el-select-dropdown__item').count();
    await page.keyboard.press('Escape');
    return n;
  });
  console.log('会话下拉选项数(应>=2):', sessOptions);
  await page.locator(".session-bar button", { hasText: "删除" }).click();
  await page.waitForTimeout(400);
  await page.locator('.el-message-box__btns .el-button--primary').click();
  await page.waitForTimeout(1000);
  const afterDelete = await page.evaluate(() => document.getElementById('app').__vue__.sessions.length);
  console.log('删除后会话数:', afterDelete);

  console.log('\nJS 错误:', errors.length ? errors.join('; ') : '（无）');
  await browser.close();
})();
