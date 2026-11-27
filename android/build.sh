#!/bin/bash
# Gera o APK do Mesa Arcana sem Gradle: precisa do Android SDK (build-tools 35 e platform 35) em $ANDROID_HOME.
set -e
cd "$(dirname "$0")"
SDK=${ANDROID_HOME:-/opt/android}; BT=$SDK/build-tools/35.0.0; JAR=$SDK/platforms/android-35/android.jar
OUT=build; rm -rf $OUT; mkdir -p $OUT/classes $OUT/assets/web/dados
cp ../mobile/index.html ../mobile/manifest.json ../mobile/icone*.png $OUT/assets/web/
cp ../mobile/dados/*.js $OUT/assets/web/dados/
$BT/aapt2 compile --dir res -o $OUT/res.zip
$BT/aapt2 link -o $OUT/base.apk -I $JAR --manifest AndroidManifest.xml -A $OUT/assets $OUT/res.zip --java $OUT/gen
javac -source 8 -target 8 -bootclasspath $JAR -classpath $JAR -d $OUT/classes $(find src $OUT/gen -name '*.java') 2>&1 | grep -v "warning\|^Note\|Picked up" || true
$BT/d8 --lib $JAR --min-api 24 --output $OUT $(find $OUT/classes -name '*.class')
cp $OUT/base.apk $OUT/semdex.apk
(cd $OUT && zip -q -j semdex.apk classes.dex)
$BT/zipalign -f -p 4 $OUT/semdex.apk $OUT/alinhado.apk
KS=${MESA_KEYSTORE:-$HOME/.mesa-arcana.keystore}
[ -f "$KS" ] || keytool -genkeypair -keystore "$KS" -storepass mesaarcana -keypass mesaarcana -alias mesa -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Mesa Arcana" >/dev/null 2>&1
$BT/apksigner sign --ks "$KS" --ks-pass pass:mesaarcana --ks-key-alias mesa --out MesaArcana.apk $OUT/alinhado.apk
$BT/apksigner verify MesaArcana.apk && ls -la MesaArcana.apk
