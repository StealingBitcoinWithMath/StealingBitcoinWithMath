from bitcoin import *
import random
import string
import subprocess

fund = ""
fund_addr = pubtoaddr(privtopub(fund)) # 1wvmTgyXDDkCrXjmNdWyAXy8xTwSHbAGM
inputs = unspent(fund_addr)

while True:
    N = 5
    password = ''.join(random.choice(string.ascii_uppercase + string.ascii_lowercase) for _ in range(N))

    priv = sha256(password)
    pub = privtopub(priv)
    addr = pubtoaddr(pub)

    h = history(addr)
    if len(h) == 0:
        break

print("""
Here's a brainwallet, derived from the passphrase

                  {}

>>> {} <<<

I'll now send some money there.

""".format(password, addr, addr))

FEE = 50000
tx = mktx(inputs[:1], [{'address': addr, 'value': inputs[0]["value"] - FEE}])
tx = sign(tx, 0, fund)
print pushtx(tx)

time.sleep(5)

subprocess.call(["open", "https://blockchain.info/address/" + addr])
