/**
 * FileFlow E2E Test v2: More precise timing and verification
 */
const { chromium } = require('playwright');

const BASE_URL = 'http://localhost:9090';
const SECRET = 'dev-secret';
const TEST_MESSAGE = 'Hello from Client A! 🚀';

async function runTest() {
  console.log('🧪 FileFlow E2E Test v2\n');

  const browser = await chromium.launch({ headless: true });
  
  // Create two separate browser contexts
  const contextA = await browser.newContext();
  const contextB = await browser.newContext();
  
  const pageA = await contextA.newPage();
  const pageB = await contextB.newPage();

  // Enable console logging
  pageA.on('console', msg => console.log(`[A] ${msg.text()}`));
  pageB.on('console', msg => console.log(`[B] ${msg.text()}`));

  try {
    // ===== Step 1: Navigate =====
    console.log('📍 Step 1: Navigate both clients');
    await pageA.goto(BASE_URL);
    await pageB.goto(BASE_URL);
    await pageA.waitForLoadState('networkidle');
    await pageB.waitForLoadState('networkidle');
    console.log('   ✓ Both loaded\n');

    // ===== Step 2: Login Client A first =====
    console.log('📍 Step 2: Login Client A');
    await pageA.fill('#secret-input', SECRET);
    await pageA.click('#secret-form button[type="submit"]');
    await pageA.waitForSelector('#view-main', { state: 'visible', timeout: 10000 });
    console.log('   ✓ Client A logged in\n');

    // ===== Step 3: Login Client B =====
    console.log('📍 Step 3: Login Client B');
    await pageB.fill('#secret-input', SECRET);
    await pageB.click('#secret-form button[type="submit"]');
    await pageB.waitForSelector('#view-main', { state: 'visible', timeout: 10000 });
    console.log('   ✓ Client B logged in\n');

    // ===== Step 4: Wait for both to show "Connected" =====
    console.log('📍 Step 4: Wait for presence to show Connected');
    
    // Wait for presence to update (both clients should see "Connected")
    await pageA.waitForFunction(() => {
      const text = document.getElementById('presence-text')?.textContent;
      return text === 'Connected';
    }, { timeout: 10000 });
    
    await pageB.waitForFunction(() => {
      const text = document.getElementById('presence-text')?.textContent;
      return text === 'Connected';
    }, { timeout: 10000 });

    const presenceA = await pageA.$eval('#presence-text', el => el.textContent);
    const presenceB = await pageB.$eval('#presence-text', el => el.textContent);
    console.log(`   Client A: "${presenceA}"`);
    console.log(`   Client B: "${presenceB}"\n`);

    // ===== Step 5: Client A sends message =====
    console.log('📍 Step 5: Client A sends message');
    console.log(`   Message: "${TEST_MESSAGE}"`);
    
    // Check send button state
    const isDisabled = await pageA.$eval('#send-button', el => el.disabled);
    console.log(`   Send button disabled before typing: ${isDisabled}`);
    
    await pageA.fill('#composer-input', TEST_MESSAGE);
    
    const isDisabledAfter = await pageA.$eval('#send-button', el => el.disabled);
    console.log(`   Send button disabled after typing: ${isDisabledAfter}`);
    
    // Click send
    await pageA.click('#send-button');
    console.log('   ✓ Send button clicked\n');

    // ===== Step 6: Wait for message to appear on Client B =====
    console.log('📍 Step 6: Wait for message on Client B');
    
    try {
      await pageB.waitForFunction((msg) => {
        const stream = document.getElementById('message-stream');
        return stream && stream.innerHTML.includes('Hello');
      }, TEST_MESSAGE, { timeout: 5000 });
      console.log('   ✓ Message received!\n');
    } catch (e) {
      console.log('   ✗ Message NOT received (timeout)\n');
    }

    // Check message stream content
    const streamA = await pageA.$eval('#message-stream', el => el.innerHTML);
    const streamB = await pageB.$eval('#message-stream', el => el.innerHTML);
    
    const msgInA = streamA.includes('Hello');
    const msgInB = streamB.includes('Hello');

    console.log(`   Client A stream has message: ${msgInA}`);
    console.log(`   Client B stream has message: ${msgInB}\n`);

    // ===== Step 7: Screenshots =====
    await pageA.screenshot({ path: '/tmp/ff-v2-A.png', fullPage: true });
    await pageB.screenshot({ path: '/tmp/ff-v2-B.png', fullPage: true });
    console.log('📍 Screenshots saved\n');

    // ===== Summary =====
    console.log('═'.repeat(50));
    console.log('📊 TEST RESULTS');
    console.log('═'.repeat(50));
    console.log(`   Both logged in:     ✓`);
    console.log(`   Both connected:     ${presenceA === 'Connected' && presenceB === 'Connected' ? '✓' : '✗'}`);
    console.log(`   Message sent (A):   ${msgInA ? '✓' : '✗'}`);
    console.log(`   Message recv (B):   ${msgInB ? '✓' : '✗'}`);
    console.log('═'.repeat(50));

    return {
      loginSuccess: true,
      presenceOK: presenceA === 'Connected' && presenceB === 'Connected',
      messageSent: msgInA,
      messageReceived: msgInB,
      coreFeatureWorks: msgInB
    };

  } catch (error) {
    console.error('❌ Error:', error.message);
    await pageA.screenshot({ path: '/tmp/ff-v2-error-A.png' }).catch(() => {});
    await pageB.screenshot({ path: '/tmp/ff-v2-error-B.png' }).catch(() => {});
    throw error;
  } finally {
    await contextA.close();
    await contextB.close();
    await browser.close();
    console.log('\n🏁 Done.');
  }
}

runTest()
  .then(result => {
    console.log('\n📋 Result:', JSON.stringify(result, null, 2));
    process.exit(result.coreFeatureWorks ? 0 : 1);
  })
  .catch(err => {
    console.error('Failed:', err.message);
    process.exit(1);
  });
