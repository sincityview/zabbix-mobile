package com.zabbix.android.monitor.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Случаи, важные для совместимости: если нормализация разойдётся, адреса из
 * сохранённых настроек перестанут попадать в API.
 */
class ZabbixUrlTest {

    @Test
    fun `дописывает схему и путь API`() {
        val cases = mapOf(
            "" to "",
            "zabbix.local" to "https://zabbix.local/api_jsonrpc.php",
            "https://zabbix.local" to "https://zabbix.local/api_jsonrpc.php",
            "https://zabbix.local/" to "https://zabbix.local/api_jsonrpc.php",
            "https://zabbix.local/zabbix" to "https://zabbix.local/zabbix/api_jsonrpc.php",
            "https://zabbix.local/zabbix/" to "https://zabbix.local/zabbix/api_jsonrpc.php",
            "http://zabbix.local/api_jsonrpc.php" to "http://zabbix.local/api_jsonrpc.php",
            "https://zabbix.local/api_jsonrpc.php?x=1#frag" to "https://zabbix.local/api_jsonrpc.php",
            "  zabbix.local/api_jsonrpc.php  " to "https://zabbix.local/api_jsonrpc.php",
        )

        cases.forEach { (input, expected) ->
            assertEquals("normalize($input)", expected, ZabbixUrl.normalize(input))
        }
    }

    @Test
    fun `определяет http без шифрования`() {
        // без схемы адрес трактуется как https — предупреждения не будет
        assertTrue(ZabbixUrl.isCleartext("http://zabbix.local"))
        assertTrue(ZabbixUrl.isCleartext("http://zabbix.local/zabbix"))
        assertFalse(ZabbixUrl.isCleartext("zabbix.local"))
        assertFalse(ZabbixUrl.isCleartext("https://zabbix.local"))
        assertFalse(ZabbixUrl.isCleartext(""))
    }
}
