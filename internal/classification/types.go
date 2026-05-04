package classification

import "sort"

// ActionTypes returns the supported action types for overrides and policy tooling.
func ActionTypes() []string {
	values := []string{
		string(ActionTypeAccessGrant),
		string(ActionTypeAccessRoleChange),
		string(ActionTypeCRMDelete),
		string(ActionTypeCRMRead),
		string(ActionTypeDataBulkExport),
		string(ActionTypeDataDelete),
		string(ActionTypeDataExport),
		string(ActionTypeDataQuery),
		string(ActionTypeDataRead),
		string(ActionTypeDataUpdate),
		string(ActionTypePaymentRefund),
		string(ActionTypeTicketComment),
		string(ActionTypeTicketCreate),
		string(ActionTypeTicketUpdate),
	}
	sort.Strings(values)
	return values
}
