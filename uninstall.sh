#!/bin/bash

BINNAME="${BINNAME:-helm}"
BINDIR="${BINDIR:-/usr/local/bin}"

echo "Uninstallation of Helm ..."
echo

sudo rm $BINDIR/$BINNAME
sudo rm "${HOME}/.config/${BINNAME}.json"

echo
echo "Uninstallation of Helm complete!"