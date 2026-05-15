#!/bin/bash
set -e

APP_NAME="SeeseoCrawler"
BUNDLE="build/${APP_NAME}.app"
# Identifier stable utilisé pour le codesign ad-hoc. macOS TCC indexe les
# permissions (Documents, Accès complet au disque...) par signature de bundle :
# sans identifier stable, chaque rebuild produit une signature aléatoire et
# wipe les perms accordées au build précédent. Avec cette ligne, les perms
# survivent aux rebuilds successifs.
CODESIGN_IDENTIFIER="fr.seeseo.crawler"

echo "==> Building frontend..."
cd frontend && npm run build --silent && cd ..
rm -rf internal/server/frontend/dist internal/server/dist
cp -r frontend/dist internal/server/frontend/dist

echo "==> Building binary..."
go build -tags desktop -o "${APP_NAME}" ./cmd/crawlobserver

echo "==> Creating ${BUNDLE}..."
rm -rf "${BUNDLE}"
mkdir -p "${BUNDLE}/Contents/MacOS"
mkdir -p "${BUNDLE}/Contents/Resources"

cp build/darwin/Info.plist "${BUNDLE}/Contents/Info.plist"
cp "${APP_NAME}" "${BUNDLE}/Contents/MacOS/${APP_NAME}"
cp build/darwin/iconfile.icns "${BUNDLE}/Contents/Resources/iconfile.icns"

echo "==> Codesigning bundle (ad-hoc, stable identifier)..."
# xattr -cr nettoie les attributs étendus (com.apple.quarantine, Finder info...)
# qui font échouer codesign avec "resource fork, Finder information, or similar
# detritus not allowed".
xattr -cr "${BUNDLE}"
# --force : remplace toute sig précédente. --sign - : ad-hoc (pas de cert Apple).
# --identifier : clé stable que TCC utilise pour retrouver les perms du bundle
# précédent (Documents, Accès complet au disque, etc.) après rebuild.
codesign --force --sign - --identifier "${CODESIGN_IDENTIFIER}" "${BUNDLE}"
codesign --verify --verbose=1 "${BUNDLE}" 2>&1 | sed 's/^/    /'

echo "==> Creating DMG..."
DMG_NAME="${APP_NAME}.dmg"
DMG_STAGING="build/dmg-staging"
rm -rf "${DMG_STAGING}" "build/${DMG_NAME}"
mkdir -p "${DMG_STAGING}"
cp -R "${BUNDLE}" "${DMG_STAGING}/"
ln -s /Applications "${DMG_STAGING}/Applications"
hdiutil create "build/${DMG_NAME}" -volname "${APP_NAME}" -srcfolder "${DMG_STAGING}" -format UDZO -ov
rm -rf "${DMG_STAGING}"

echo "==> Done! DMG: build/${DMG_NAME}"
echo "    Or run directly: open ${BUNDLE}"
