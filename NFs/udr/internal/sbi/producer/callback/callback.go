package callback

import (
	"context"
	"runtime/debug"

	"github.com/free5gc/openapi/Nudr_DataRepository"
	"github.com/free5gc/openapi/models"
	udr_context "github.com/free5gc/udr/internal/context"
	"github.com/free5gc/udr/internal/logger"
)

func SendOnDataChangeNotify(ueId string, notifyItems []models.NotifyItem) {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			logger.HttpLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}
	}()

	udrSelf := udr_context.UDR_Self()
	configuration := Nudr_DataRepository.NewConfiguration()
	client := Nudr_DataRepository.NewAPIClient(configuration)

	for _, subscriptionDataSubscription := range udrSelf.SubscriptionDataSubscriptions {
		if ueId == subscriptionDataSubscription.UeId {
			onDataChangeNotifyUrl := subscriptionDataSubscription.CallbackReference

			dataChangeNotify := models.DataChangeNotify{}
			dataChangeNotify.UeId = ueId
			dataChangeNotify.OriginalCallbackReference = []string{subscriptionDataSubscription.OriginalCallbackReference}
			dataChangeNotify.NotifyItems = notifyItems
			httpResponse, err := client.DataChangeNotifyCallbackDocumentApi.OnDataChangeNotify(context.TODO(),
				onDataChangeNotifyUrl, dataChangeNotify)
			if err != nil {
				if httpResponse == nil {
					logger.HttpLog.Errorln(err.Error())
				} else if err.Error() != httpResponse.Status {
					logger.HttpLog.Errorln(err.Error())
				}
			}
		}
	}
}

func SendPolicyDataChangeNotification(policyDataChangeNotification models.PolicyDataChangeNotification) {
	defer func() {
		if p := recover(); p != nil {
			// Print stack for panic to log. Fatalf() will let program exit.
			logger.HttpLog.Fatalf("panic: %v\n%s", p, string(debug.Stack()))
		}
	}()

	udrSelf := udr_context.UDR_Self()

	for _, policyDataSubscription := range udrSelf.PolicyDataSubscriptions {
		policyDataChangeNotificationUrl := policyDataSubscription.NotificationUri

		configuration := Nudr_DataRepository.NewConfiguration()
		client := Nudr_DataRepository.NewAPIClient(configuration)
		httpResponse, err := client.PolicyDataChangeNotificationCallbackDocumentApi.PolicyDataChangeNotification(
			context.TODO(), policyDataChangeNotificationUrl, policyDataChangeNotification)
		if err != nil {
			if httpResponse == nil {
				logger.HttpLog.Errorln(err.Error())
			} else if err.Error() != httpResponse.Status {
				logger.HttpLog.Errorln(err.Error())
			}
		}
	}
}
