import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Версия приходит из CI (тег v1.2.3 + номер прогона), локально — заглушка.
// providers вместо System.getenv, чтобы не ломать configuration-cache.
val appVersionName = providers.environmentVariable("VERSION_NAME").orElse("0.1.0-dev")
val appVersionCode = providers.environmentVariable("VERSION_CODE").orElse("1")

// Ключ подписи монтируется в CI из секрета (base64 → файл). Локально его может
// не быть — тогда release-сборка подписывается debug-ключом, чтобы `assembleRelease`
// работал без секретов.
val existingKeystore = providers.environmentVariable("KEYSTORE_FILE")
    .orNull
    ?.let(::file)
    ?.takeIf { it.exists() }

android {
    namespace = "com.zabbix.android.monitor"
    compileSdk = 36

    defaultConfig {
        // applicationId не менялся с первых версий: при том же ключе подписи
        // приложение обновляется поверх установленного, а не встаёт вторым.
        applicationId = "com.zabbix.android.monitor"
        minSdk = 26
        targetSdk = 36

        versionName = appVersionName.get()
        versionCode = appVersionCode.get().toInt()
    }

    signingConfigs {
        existingKeystore?.let { ks ->
            create("release") {
                storeFile = ks
                storePassword = providers.environmentVariable("KEYSTORE_PASS").orNull
                keyAlias = providers.environmentVariable("KEY_ALIAS").orNull
                keyPassword = providers.environmentVariable("KEY_PASS").orNull
            }
        }
    }

    buildTypes {
        release {
            // R8 пока выключен: kotlinx.serialization опирается на
            // сгенерированные сериализаторы, и если правила сжатия их снесут,
            // приложение упадёт уже на устройстве. Включим, когда появятся
            // прогоны на живом телефоне — APK и без сжатия небольшой.
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }

    lint {
        // собираем APK даже при предупреждениях, но падаем на ошибках
        abortOnError = true
        checkDependencies = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget = JvmTarget.JVM_17
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
}
