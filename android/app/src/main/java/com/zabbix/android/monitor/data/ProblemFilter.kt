package com.zabbix.android.monitor.data

/**
 * Что показывать в списке: все проблемы ([All]) или только выбранная важность.
 *
 * Это именно отображение, а не сортировка: при выбранном уровне в списке
 * остаются только проблемы этого уровня, а порядок внутри — серверный
 * (свежие сверху).
 */
enum class ProblemFilter(val level: Int?) {
    All(null),
    Disaster(5),
    High(4),
    Average(3),
    Warning(2),
    Information(1),
    NotClassified(0),
    ;

    companion object {
        fun fromName(name: String?): ProblemFilter =
            entries.firstOrNull { it.name == name } ?: All
    }
}

/** Оставляет проблемы выбранной важности; [ProblemFilter.All] возвращает всё. */
fun filterProblems(problems: List<Problem>, filter: ProblemFilter): List<Problem> {
    val level = filter.level ?: return problems
    return problems.filter { it.severityLevel == level }
}
