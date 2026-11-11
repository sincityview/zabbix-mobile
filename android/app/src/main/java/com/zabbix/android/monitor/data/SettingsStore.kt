package com.zabbix.android.monitor.data

import android.content.Context

/** Настройки подключения. Токен хранится в приватных prefs приложения. */
data class MonitorSettings(
    val url: String = "",
    val token: String = "",
    val allowInsecureTls: Boolean = false,
) {
    val isConfigured: Boolean
        get() = ZabbixUrl.normalize(url).isNotEmpty() && token.isNotBlank()

    /** Идемпотентное представление для ключа кеша: сервер + токен. */
    fun cacheKey(): String = ZabbixUrl.normalize(url) + "|" + token
}

class SettingsStore(context: Context) {

    private val prefs =
        context.applicationContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    fun load(): MonitorSettings = MonitorSettings(
        url = prefs.getString(KEY_URL, "").orEmpty(),
        token = prefs.getString(KEY_TOKEN, "").orEmpty(),
        allowInsecureTls = prefs.getBoolean(KEY_INSECURE_TLS, false),
    )

    fun save(settings: MonitorSettings) {
        prefs.edit()
            .putString(KEY_URL, ZabbixUrl.normalize(settings.url))
            .putString(KEY_TOKEN, settings.token.trim())
            .putBoolean(KEY_INSECURE_TLS, settings.allowInsecureTls)
            .apply()
    }

    fun clear() {
        prefs.edit().clear().apply()
    }

    private companion object {
        const val PREFS_NAME = "monitor_settings"
        const val KEY_URL = "url"
        const val KEY_TOKEN = "token"
        const val KEY_INSECURE_TLS = "insecure_tls"
    }
}
