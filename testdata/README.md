# testdata

Not committed to the repo, fingerprint images from a real (even if public
research) dataset shouldn't sit in git history. Fetch it locally:

```
./scripts/fetch-testdata.sh
```

This pulls **FVC2002 DB1_B** into `testdata/fvc2002-db1/` — 10 fingers,
8 impressions each, optical sensor at 500dpi. This is the exact dataset
SourceAFIS's own tutorial recommends for trying the library out, and the
500dpi matches `SCANNER_DPI` in `MatcherServiceImpl.java` with no unit
conversion needed.

## Using it with the CLI

Files are named `<finger>_<impression>.tif`, fingers are numbered
101-110 (not 1-10). Same finger, different impressions, should verify
as a match. Different fingers should not.

```
cd go-client

# enroll one impression of finger 101
go run ./cmd/biometric-cli enroll \
  --scan ../testdata/fvc2002-db1/101_1.tif \
  --out ../testdata/template-101.bin

# verify against a different impression of the SAME finger, expect match: true
go run ./cmd/biometric-cli verify \
  --scan ../testdata/fvc2002-db1/101_2.tif \
  --template ../testdata/template-101.bin

# verify against a DIFFERENT finger, expect match: false
go run ./cmd/biometric-cli verify \
  --scan ../testdata/fvc2002-db1/102_1.tif \
  --template ../testdata/template-101.bin
```

## If you swap in a different FVC database

Check that database's sensor DPI before assuming it'll just work.
FVC2000/2002/2004 span several different sensors and not all of them are
500dpi, a mismatch here doesn't error, it just silently degrades match
quality, since SourceAFIS trusts whatever DPI you tell it rather than
reading it from the image itself.
