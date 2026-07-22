#!/usr/bin/env bash
set -euo pipefail

# FVC2002 DB1_B: 10 fingers x 8 impressions each, captured on an optical
# sensor at 500dpi. Picked over DB2/DB3 specifically because 500dpi
# matches SCANNER_DPI in MatcherServiceImpl.java without any conversion,
# using a different DB here means updating that constant too, not just
# swapping the data.
FVC_URL="http://bias.csr.unibo.it/fvc2002/Downloads/DB1_B.zip"
DEST_DIR="$(dirname "$0")/../testdata/fvc2002-db1"

mkdir -p "$DEST_DIR"

echo "Downloading FVC2002 DB1_B into $DEST_DIR"
curl -L "$FVC_URL" -o /tmp/fvc2002-db1.zip

echo "Unzipping"
unzip -o /tmp/fvc2002-db1.zip -d "$DEST_DIR"
rm /tmp/fvc2002-db1.zip

echo "Done. Files are named <finger>_<impression>.tif, e.g. 1_1.tif and"
echo "1_2.tif are two different impressions of the same finger 1, useful"
echo "for a real verify() match test. 1_1.tif vs 2_1.tif is a genuine"
echo "non-match test."