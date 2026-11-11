package com.zabbix.android.monitor.data

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** Форматирование unix-времени из ответов API. */
object ZabbixTime {

    const val DATE_TIME = "dd.MM HH:mm:ss"
    const val TIME_OF_DAY = "HH:mm"

    fun format(clock: String, pattern: String = DATE_TIME): String {
        val seconds = clock.toLongOrNull() ?: return clock
        return SimpleDateFormat(pattern, Locale.getDefault()).format(Date(seconds * 1000))
    }
}
