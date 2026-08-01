async function request(url: string) {
    return (await fetch(url, {}))
}

console.log(await request())