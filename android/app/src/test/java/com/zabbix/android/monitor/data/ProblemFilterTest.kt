package com.zabbix.android.monitor.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Test

class ProblemFilterTest {

    private fun problem(eventid: String, severity: Int) = Problem(
        eventid = eventid,
        objectid = "t$eventid",
        severity = severity.toString(),
        name = "P$eventid",
    )

    private val problems = listOf(
        problem("5", 1),
        problem("4", 5),
        problem("3", 2),
        problem("2", 5),
        problem("1", 0),
    )

    @Test
    fun `по умолчанию показывается всё в порядке сервера`() {
        val filtered = filterProblems(problems, ProblemFilter.All)

        assertEquals(listOf("5", "4", "3", "2", "1"), filtered.map { it.eventid })
        assertSame("список не должен копироваться зря", problems, filtered)
    }

    @Test
    fun `выбранная важность оставляет только её`() {
        assertEquals(
            listOf("4", "2"),
            filterProblems(problems, ProblemFilter.Disaster).map { it.eventid },
        )
        assertEquals(
            listOf("3"),
            filterProblems(problems, ProblemFilter.Warning).map { it.eventid },
        )
        assertEquals(
            listOf("1"),
            filterProblems(problems, ProblemFilter.NotClassified).map { it.eventid },
        )
    }

    @Test
    fun `порядок внутри важности остаётся серверным`() {
        // сервер уже отдаёт свежие сверху — фильтр его не переставляет
        assertEquals(
            listOf("4", "2"),
            filterProblems(problems, ProblemFilter.Disaster).map { it.eventid },
        )
    }

    @Test
    fun `нет проблем такой важности — пустой список`() {
        assertEquals(
            emptyList<Problem>(),
            filterProblems(problems, ProblemFilter.Average),
        )
    }

    @Test
    fun `неизвестное значение настройки читается как по умолчанию`() {
        assertEquals(ProblemFilter.All, ProblemFilter.fromName(null))
        assertEquals(ProblemFilter.All, ProblemFilter.fromName("мусор"))
        assertEquals(ProblemFilter.High, ProblemFilter.fromName("High"))
    }
}
