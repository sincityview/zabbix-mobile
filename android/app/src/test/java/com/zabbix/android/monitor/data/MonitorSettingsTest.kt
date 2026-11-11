package com.zabbix.android.monitor.data

import org.junit.Assert.assertEquals
import org.junit.Test

class MonitorSettingsTest {

    // Регрессия: экран собирал настройки конструктором и терял поле лимита —
    // «Лимит отображения проблем» не применялся на главной.
    @Test
    fun `copy не теряет лимит и интервал`() {
        val initial = MonitorSettings(limit = 7, intervalSeconds = 15)

        val saved = initial.copy(
            url = ZabbixUrl.normalize("zabbix.local"),
            token = "tok",
        )

        assertEquals(7, saved.limit)
        assertEquals(15, saved.intervalSeconds)
    }

    @Test
    fun `normalized приводит адрес, лимит и интервал`() {
        val settings = MonitorSettings(
            url = "  zabbix.local  ",
            token = "  tok  ",
            limit = 42,
            intervalSeconds = 30,
        )

        val normalized = settings.normalized()

        assertEquals("https://zabbix.local/api_jsonrpc.php", normalized.url)
        assertEquals("tok", normalized.token)
        assertEquals(42, normalized.limit)
        assertEquals(30, normalized.intervalSeconds)
    }

    @Test
    fun `normalized зажимает значения в допустимые границы`() {
        assertEquals(1, MonitorSettings(limit = 0).normalized().limit)
        assertEquals(1, MonitorSettings(limit = -5).normalized().limit)
        assertEquals(ZabbixApi.MAX_LIMIT, MonitorSettings(limit = 99_999).normalized().limit)

        assertEquals(
            MonitorSettings.MIN_INTERVAL_SECONDS,
            MonitorSettings(intervalSeconds = 1).normalized().intervalSeconds,
        )
        assertEquals(
            MonitorSettings.MAX_INTERVAL_SECONDS,
            MonitorSettings(intervalSeconds = 99_999).normalized().intervalSeconds,
        )
    }

    @Test
    fun `по умолчанию лимит 100 и интервал 60 секунд`() {
        val settings = MonitorSettings()

        assertEquals(ZabbixApi.DEFAULT_LIMIT, settings.safeLimit())
        assertEquals(MonitorSettings.DEFAULT_INTERVAL_SECONDS, settings.safeIntervalSeconds())
        assertEquals(100, settings.limit)
        assertEquals(60, settings.intervalSeconds)
    }
}
