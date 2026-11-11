package com.zabbix.android.monitor.data

import android.content.Context

/** Настройки подключения. Токен хранится в приватных prefs приложения. */
data class MonitorSettings(
    val url: String = "",
    val token: String = "",
    val allowInsecureTls: Boolean = false,
    /** Сколько проблем показывать в списке. */
    val limit: Int = ZabbixApi.DEFAULT_LIMIT,
    /** Интервал автообновления, секунды. */
    val intervalSeconds: Int = DEFAULT_INTERVAL_SECONDS,
) {
    val isConfigured: Boolean
        get() = ZabbixUrl.normalize(url).isNotEmpty() && token.isNotBlank()

    /** Идемпотентное представление для ключа кеша: сервер + токен. */
    fun cacheKey(): String = ZabbixUrl.normalize(url) + "|" + token

    /** Лимит в допустимых границах: мусор в настройках не должен ломать экран. */
    fun safeLimit(): Int = limit.coerceIn(1, ZabbixApi.MAX_LIMIT)

    /** Интервал автообновления в допустимых границах, секунды. */
    fun safeIntervalSeconds(): Int = intervalSeconds.coerceIn(MIN_INTERVAL_SECONDS, MAX_INTERVAL_SECONDS)

    /**
     * Приведение к каноническому виду.
     *
     * Через `copy`, а не конструктором: раньше экран собирал настройки заново
     * и терял поле лимита — «Лимит отображения» не сохранялся.
     */
    fun normalized(): MonitorSettings = copy(
        url = ZabbixUrl.normalize(url),
        token = token.trim(),
        limit = safeLimit(),
        intervalSeconds = safeIntervalSeconds(),
    )

    companion object {
        const val DEFAULT_INTERVAL_SECONDS = 60
        const val MIN_INTERVAL_SECONDS = 10
        const val MAX_INTERVAL_SECONDS = 3600
    }
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
        intervalSeconds = prefs.getInt(KEY_INTERVAL, MonitorSettings.DEFAULT_INTERVAL_SECONDS)
            .coerceIn(MonitorSettings.MIN_INTERVAL_SECONDS, MonitorSettings.MAX_INTERVAL_SECONDS),
    )

    fun save(settings: MonitorSettings) {
        val st = settings.normalized()
        prefs.edit()
            .putString(KEY_URL, st.url)
            .putString(KEY_TOKEN, st.token)
            .putBoolean(KEY_INSECURE_TLS, st.allowInsecureTls)
            .putInt(KEY_LIMIT, st.safeLimit())
            .putInt(KEY_INTERVAL, st.safeIntervalSeconds())
            .apply()
    }

    /** Фильтр показа по важности — отдельный ключ, остальных настроек не касается. */
    fun loadFilter(): ProblemFilter =
        ProblemFilter.fromName(prefs.getString(KEY_FILTER, null))

    fun saveFilter(filter: ProblemFilter) {
        prefs.edit().putString(KEY_FILTER, filter.name).apply()
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
        const val KEY_INTERVAL = "interval_seconds"
        const val KEY_FILTER = "severity_filter"
    }
}
