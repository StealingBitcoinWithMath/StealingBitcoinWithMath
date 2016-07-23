#! /usr/bin/env python2
from bitcoin import *
import time
import hashlib
import sys

FEE = 50000
RETURN_ADDR = "1wvmTgyXDDkCrXjmNdWyAXy8xTwSHbAGM"

fixed_k = decode(hashlib.sha256(random_electrum_seed()).digest(), 256)
print " * Using a fixed nonce: %x" % fixed_k

def deterministic_generate_k(a, b):
    return fixed_k # Uh oh

def ecdsa_raw_sign(msghash, priv):
    z = hash_to_int(msghash)
    k = deterministic_generate_k(msghash, priv)
    r, y = fast_multiply(G, k)
    s = inv(k, N) * (z + r*decode_privkey(priv)) % N
    v, r, s = 27+((y % 2) ^ (0 if s * 2 < N else 1)), r, s if s * 2 < N else N - s
    if 'compressed' in get_privkey_format(priv):
        v += 4
    return v, r, s

def ecdsa_tx_sign(tx, priv, hashcode=SIGHASH_ALL):
    rawsig = ecdsa_raw_sign(bin_txhash(tx, hashcode), priv)
    return der_encode_sig(*rawsig)+encode(hashcode, 16, 2)

def sign(tx, i, priv, hashcode=SIGHASH_ALL):
    if (not is_python2 and isinstance(re, bytes)) or not re.match('^[0-9a-fA-F]*$', tx):
        return binascii.unhexlify(sign(safe_hexlify(tx), i, priv))
    if len(priv) <= 33:
        priv = safe_hexlify(priv)
    pub = privkey_to_pubkey(priv)
    address = pubkey_to_address(pub)
    signing_tx = signature_form(tx, i, mk_pubkey_script(address), hashcode)
    sig = ecdsa_tx_sign(signing_tx, priv, hashcode)
    txobj = deserialize(tx)
    txobj["ins"][i]["script"] = serialize_script([sig, pub])
    return serialize(txobj)

priv = sys.argv[1]
print " * Private key is", priv

addr = pubtoaddr(privtopub(priv))
print " * Address is", addr

print " * Waiting to receive 3 deposits...",
sys.stdout.flush()
while True:
    inputs = unspent(addr)
    if len(inputs) >= 3:
        print ""
        break
    time.sleep(5)
    print ".",
    sys.stdout.flush()

value = inputs[-1]["value"] + inputs[-2]["value"] - FEE
print " * Sending 2 inputs back... ",
tx = mktx(inputs[-2:], [{'address': RETURN_ADDR, 'value': value}])
tx = sign(tx, 0, priv)
tx = sign(tx, 1, priv)
print pushtx(tx)

print "https://blockchain.info/address/" + addr
