package classification

import "sort"

// ActionTypes returns the supported action types for overrides and policy tooling.
func ActionTypes() []string {
	values := []string{
		"ACCESS.GRANT",
		"ACCESS.ROLE_CHANGE",
		"CRM.DELETE",
		"CRM.READ",
		"DATA.BULK_EXPORT",
		"DATA.DELETE",
		"DATA.EXPORT",
		"DATA.QUERY",
		"DATA.READ",
		"DATA.UPDATE",
		"PAYMENT.REFUND",
		"TICKET.COMMENT",
		"TICKET.CREATE",
		"TICKET.UPDATE",
	}
	sort.Strings(values)
	return values
}
