package com.zabbix.android.monitor.data

import android.content.Context

/** Настройки подключения. Токен хранится в приватных prefs приложения. */
data class MonitorSettings(
    val url: String = "",
    val token: String = "",
    val allowInsecureTls: Boolean = false,
    /** Сколько проблем показывать в списке. */
    val limit: Int = ZabbixApi.DEFAULT_LIMIT,
) {
    val isConfigured: Boolean
        get() = ZabbixUrl.normalize(url).isNotEmpty() && token.isNotBlank()

    /** Идемпотентное представление для ключа кеша: сервер + токен. */
    fun cacheKey(): String = ZabbixUrl.normalize(url) + "|" + token

    /** Лимит в допустимых границах: мусор в настройках не должен ломать экран. */
    fun safeLimit(): Int = limit.coerceIn(1, ZabbixApi.MAX_LIMIT)
}

class SettingsStore(context: Context) {

    private val prefs =
        context.applicationContext.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    fun load(): MonitorSettings = MonitorSettings(
        url = prefs.getString(KEY_URL, "").orEmpty(),
        token = prefs.getString(KEY_TOKEN, "").orEmpty(),
        allowInsecureTls = prefs.getBoolean(KEY_INSECURE_TLS, false),
        limit = prefs.getInt(KEY_LIMIT, ZabbixApi.DEFAULT_LIMIT)
            .coerceIn(1, ZabbixApi.MAX_LIMIT),
    )

    fun save(settings: MonitorSettings) {
        prefs.edit()
            .putString(KEY_URL, ZabbixUrl.normalize(settings.url))
            .putString(KEY_TOKEN, settings.token.trim())
            .putBoolean(KEY_INSECURE_TLS, settings.allowInsecureTls)
            .putInt(KEY_LIMIT, settings.safeLimit())
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
        const val KEY_LIMIT = "limit"
    }
}
