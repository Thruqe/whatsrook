import fs from 'fs'
import readline from 'readline'

const BASE_URL = "https://embers-0kn7.onrender.com"
const COOKIES_DOMAIN = "youtube.com"
const COOKIES_FILE = "./cookies.txt"

const rl = readline.createInterface({ input: process.stdin, output: process.stdout })
const ask = (q: string): Promise<string> => new Promise((resolve) => rl.question(q, resolve))

async function setCookies() {
    const cookieContent = fs.readFileSync(COOKIES_FILE, 'utf-8')
    const res = await fetch(`${BASE_URL}/cookies`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ domain: COOKIES_DOMAIN, cookies: cookieContent }),
    })
    const data = await res.json()
    if (data.error) throw new Error(`Failed to set cookies: ${data.message}`)
}

async function main() {
    console.log('[cookies] loading...')
    await setCookies()
    console.log('[cookies] ready\n')

    const url = await ask('Video URL: ')
    console.log('\nFetching raw info...\n')

    const res = await fetch(`${BASE_URL}/download?url=${encodeURIComponent(url.trim())}`)
    const raw = await res.json()

    // dump everything to a file so we can actually inspect it
    fs.writeFileSync('./last_response.json', JSON.stringify(raw, null, 2))
    console.log(`Full response saved to ./last_response.json`)

    console.log('\nTop-level keys:', Object.keys(raw))
    if (raw.data) {
        console.log('data keys:', Object.keys(raw.data))
        console.log('formats count:', raw.data.formats?.length ?? 'no formats field')
        if (raw.data.formats?.length) {
            console.log('\nFirst format entry (raw):')
            console.log(JSON.stringify(raw.data.formats[0], null, 2))
            console.log('\nLast format entry (raw):')
            console.log(JSON.stringify(raw.data.formats[raw.data.formats.length - 1], null, 2))
        }
    }

    rl.close()
}

main().catch((err) => {
    console.error('Error:', err.message)
    rl.close()
    process.exit(1)
})