package producer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/bronze1man/radius"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	ausf_context "github.com/free5gc/ausf/internal/context"
	"github.com/free5gc/ausf/internal/logger"
	"github.com/free5gc/openapi/models"
	"github.com/free5gc/util/httpwrapper"
	"github.com/free5gc/util/ueauth"
)

func HandleEapAuthComfirmRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.Auth5gAkaComfirmLog.Infof("EapAuthComfirmRequest")

	updateEapSession := request.Body.(models.EapSession)
	eapSessionID := request.Params["authCtxId"]

	response, problemDetails := EapAuthComfirmRequestProcedure(updateEapSession, eapSessionID)

	if response != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleAuth5gAkaComfirmRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.Auth5gAkaComfirmLog.Infof("Auth5gAkaComfirmRequest")
	updateConfirmationData := request.Body.(models.ConfirmationData)
	ConfirmationDataResponseID := request.Params["authCtxId"]

	response, problemDetails := Auth5gAkaComfirmRequestProcedure(updateConfirmationData, ConfirmationDataResponseID)
	if response != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleUeAuthPostRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.UeAuthPostLog.Infof("HandleUeAuthPostRequest")
	updateAuthenticationInfo := request.Body.(models.AuthenticationInfo)

	response, locationURI, problemDetails := UeAuthPostRequestProcedure(updateAuthenticationInfo)
	respHeader := make(http.Header)
	respHeader.Set("Location", locationURI)

	if response != nil {
		return httpwrapper.NewResponse(http.StatusCreated, respHeader, response)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.Status), nil, problemDetails)
	}
	problemDetails = &models.ProblemDetails{
		Status: http.StatusForbidden,
		Cause:  "UNSPECIFIED",
	}
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

// func UeAuthPostRequestProcedure(updateAuthenticationInfo models.AuthenticationInfo) (
//    response *models.UeAuthenticationCtx, locationURI string, problemDetails *models.ProblemDetails) {
func UeAuthPostRequestProcedure(updateAuthenticationInfo models.AuthenticationInfo) (*models.UeAuthenticationCtx,
	string, *models.ProblemDetails,
) {
	var responseBody models.UeAuthenticationCtx
	var authInfoReq models.AuthenticationInfoRequest

	supiOrSuci := updateAuthenticationInfo.SupiOrSuci

	snName := updateAuthenticationInfo.ServingNetworkName
	servingNetworkAuthorized := ausf_context.IsServingNetworkAuthorized(snName)
	if !servingNetworkAuthorized {
		var problemDetails models.ProblemDetails
		problemDetails.Cause = "SERVING_NETWORK_NOT_AUTHORIZED"
		problemDetails.Status = http.StatusForbidden
		logger.UeAuthPostLog.Infoln("403 forbidden: serving network NOT AUTHORIZED")
		return nil, "", &problemDetails
	}
	logger.UeAuthPostLog.Infoln("Serving network authorized")

	responseBody.ServingNetworkName = snName
	authInfoReq.ServingNetworkName = snName
	self := ausf_context.GetSelf()
	authInfoReq.AusfInstanceId = self.GetSelfID()

	var lastEapID uint8
	if updateAuthenticationInfo.ResynchronizationInfo != nil {
		logger.UeAuthPostLog.Warningln("Auts: ", updateAuthenticationInfo.ResynchronizationInfo.Auts)
		ausfCurrentSupi := ausf_context.GetSupiFromSuciSupiMap(supiOrSuci)
		logger.UeAuthPostLog.Warningln(ausfCurrentSupi)
		ausfCurrentContext := ausf_context.GetAusfUeContext(ausfCurrentSupi)
		logger.UeAuthPostLog.Warningln(ausfCurrentContext.Rand)
		if updateAuthenticationInfo.ResynchronizationInfo.Rand == "" {
			updateAuthenticationInfo.ResynchronizationInfo.Rand = ausfCurrentContext.Rand
		}
		logger.UeAuthPostLog.Warningln("Rand: ", updateAuthenticationInfo.ResynchronizationInfo.Rand)
		authInfoReq.ResynchronizationInfo = updateAuthenticationInfo.ResynchronizationInfo
		lastEapID = ausfCurrentContext.EapID
	}

	udmUrl := getUdmUrl(self.NrfUri)
	client := createClientToUdmUeau(udmUrl)
	authInfoResult, rsp, err := client.GenerateAuthDataApi.GenerateAuthData(context.Background(), supiOrSuci, authInfoReq)
	if err != nil {
		logger.UeAuthPostLog.Infoln(err.Error())
		var problemDetails models.ProblemDetails
		if authInfoResult.AuthenticationVector == nil {
			problemDetails.Cause = "AV_GENERATION_PROBLEM"
		} else {
			problemDetails.Cause = "UPSTREAM_SERVER_ERROR"
		}
		problemDetails.Status = http.StatusInternalServerError
		return nil, "", &problemDetails
	}
	defer func() {
		if rspCloseErr := rsp.Body.Close(); rspCloseErr != nil {
			logger.UeAuthPostLog.Errorf("GenerateAuthDataApi response body cannot close: %+v", rspCloseErr)
		}
	}()

	ueid := authInfoResult.Supi
	ausfUeContext := ausf_context.NewAusfUeContext(ueid)
	ausfUeContext.ServingNetworkName = snName
	ausfUeContext.AuthStatus = models.AuthResult_ONGOING
	ausfUeContext.UdmUeauUrl = udmUrl
	ausf_context.AddAusfUeContextToPool(ausfUeContext)

	logger.UeAuthPostLog.Infof("Add SuciSupiPair (%s, %s) to map.\n", supiOrSuci, ueid)
	ausf_context.AddSuciSupiPairToMap(supiOrSuci, ueid)

	locationURI := self.Url + "/nausf-auth/v1/ue-authentications/" + supiOrSuci
	putLink := locationURI
	if authInfoResult.AuthType == models.AuthType__5_G_AKA {
		logger.UeAuthPostLog.Infoln("Use 5G AKA auth method")
		putLink += "/5g-aka-confirmation"

		// Derive HXRES* from XRES*
		concat := authInfoResult.AuthenticationVector.Rand + authInfoResult.AuthenticationVector.XresStar
		var hxresStarBytes []byte
		if bytes, err := hex.DecodeString(concat); err != nil {
			logger.Auth5gAkaComfirmLog.Errorf("decode concat error: %+v", err)
			var problemDetails models.ProblemDetails
			problemDetails.Title = "Concat Decode Problem"
			problemDetails.Cause = "CONCAT_DECODE_PROBLEM"
			problemDetails.Detail = err.Error()
			problemDetails.Status = http.StatusInternalServerError
			return nil, "", &problemDetails
		} else {
			hxresStarBytes = bytes
		}
		hxresStarAll := sha256.Sum256(hxresStarBytes)
		hxresStar := hex.EncodeToString(hxresStarAll[16:]) // last 128 bits
		logger.Auth5gAkaComfirmLog.Infof("XresStar = %x\n", authInfoResult.AuthenticationVector.XresStar)

		// Derive Kseaf from Kausf
		Kausf := authInfoResult.AuthenticationVector.Kausf
		var KausfDecode []byte
		if ausfDecode, err := hex.DecodeString(Kausf); err != nil {
			logger.Auth5gAkaComfirmLog.Errorf("decode Kausf failed: %+v", err)
			var problemDetails models.ProblemDetails
			problemDetails.Title = "Kausf Decode Problem"
			problemDetails.Cause = "KAUSF_DECODE_PROBLEM"
			problemDetails.Detail = err.Error()
			problemDetails.Status = http.StatusInternalServerError
			return nil, "", &problemDetails
		} else {
			KausfDecode = ausfDecode
		}
		P0 := []byte(snName)
		Kseaf, err := ueauth.GetKDFValue(KausfDecode, ueauth.FC_FOR_KSEAF_DERIVATION, P0, ueauth.KDFLen(P0))
		if err != nil {
			logger.Auth5gAkaComfirmLog.Errorf("GetKDFValue failed: %+v", err)
			var problemDetails models.ProblemDetails
			problemDetails.Title = "Kseaf Derivation Problem"
			problemDetails.Cause = "KSEAF_DERIVATION_PROBLEM"
			problemDetails.Detail = err.Error()
			problemDetails.Status = http.StatusInternalServerError
			return nil, "", &problemDetails
		}
		ausfUeContext.XresStar = authInfoResult.AuthenticationVector.XresStar
		ausfUeContext.Kausf = Kausf
		ausfUeContext.Kseaf = hex.EncodeToString(Kseaf)
		ausfUeContext.Rand = authInfoResult.AuthenticationVector.Rand

		var av5gAka models.Av5gAka
		av5gAka.Rand = authInfoResult.AuthenticationVector.Rand
		av5gAka.Autn = authInfoResult.AuthenticationVector.Autn
		av5gAka.HxresStar = hxresStar
		responseBody.Var5gAuthData = av5gAka

		linksValue := models.LinksValueSchema{Href: putLink}
		responseBody.Links = make(map[string]models.LinksValueSchema)
		responseBody.Links["5g-aka"] = linksValue
	} else if authInfoResult.AuthType == models.AuthType_EAP_AKA_PRIME {
		logger.UeAuthPostLog.Infoln("Use EAP-AKA' auth method")
		putLink += "/eap-session"

		var identity string
		// TODO support more SUPI type
		if ueid[:4] == "imsi" {
			if !self.EapAkaSupiImsiPrefix {
				// 33.501 v15.9.0 or later
				identity = ueid[5:]
			} else {
				// 33.501 v15.8.0 or earlier
				identity = ueid
			}
		}
		ikPrime := authInfoResult.AuthenticationVector.IkPrime
		ckPrime := authInfoResult.AuthenticationVector.CkPrime
		RAND := authInfoResult.AuthenticationVector.Rand
		AUTN := authInfoResult.AuthenticationVector.Autn
		XRES := authInfoResult.AuthenticationVector.Xres
		ausfUeContext.XRES = XRES

		ausfUeContext.Rand = authInfoResult.AuthenticationVector.Rand

		_, K_aut, _, _, EMSK := eapAkaPrimePrf(ikPrime, ckPrime, identity)
		logger.EapAuthComfirmLog.Tracef("K_aut: %x", K_aut)
		ausfUeContext.K_aut = hex.EncodeToString(K_aut)
		Kausf := EMSK[0:32]
		ausfUeContext.Kausf = hex.EncodeToString(Kausf)
		P0 := []byte(snName)
		Kseaf, err := ueauth.GetKDFValue(Kausf, ueauth.FC_FOR_KSEAF_DERIVATION, P0, ueauth.KDFLen(P0))
		if err != nil {
			logger.EapAuthComfirmLog.Errorf("GetKDFValue failed: %+v", err)
		}
		ausfUeContext.Kseaf = hex.EncodeToString(Kseaf)

		var eapPkt radius.EapPacket
		eapPkt.Code = radius.EapCode(1)
		if updateAuthenticationInfo.ResynchronizationInfo == nil {
			rand.Seed(time.Now().Unix())
			randIdentifier := rand.Intn(256)
			ausfUeContext.EapID = uint8(randIdentifier)
		} else {
			ausfUeContext.EapID = lastEapID + 1
		}
		eapPkt.Identifier = ausfUeContext.EapID
		eapPkt.Type = radius.EapType(50) // according to RFC5448 6.1

		var eapAKAHdr, atRand, atAutn, atKdf, atKdfInput, atMAC string
		eapAKAHdrBytes := make([]byte, 3) // RFC4187 8.1
		eapAKAHdrBytes[0] = ausf_context.AKA_CHALLENGE_SUBTYPE
		eapAKAHdr = string(eapAKAHdrBytes)
		if atRandTmp, err := EapEncodeAttribute("AT_RAND", RAND); err != nil {
			logger.EapAuthComfirmLog.Errorf("EAP encode RAND failed: %+v", err)
		} else {
			atRand = atRandTmp
		}
		if atAutnTmp, err := EapEncodeAttribute("AT_AUTN", AUTN); err != nil {
			logger.EapAuthComfirmLog.Errorf("EAP encode AUTN failed: %+v", err)
		} else {
			atAutn = atAutnTmp
		}
		if atKdfTmp, err := EapEncodeAttribute("AT_KDF", snName); err != nil {
			logger.EapAuthComfirmLog.Errorf("EAP encode KDF failed: %+v", err)
		} else {
			atKdf = atKdfTmp
		}
		if atKdfInputTmp, err := EapEncodeAttribute("AT_KDF_INPUT", snName); err != nil {
			logger.EapAuthComfirmLog.Errorf("EAP encode KDF failed: %+v", err)
		} else {
			atKdfInput = atKdfInputTmp
		}
		if atMACTmp, err := EapEncodeAttribute("AT_MAC", ""); err != nil {
			logger.EapAuthComfirmLog.Errorf("EAP encode MAC failed: %+v", err)
		} else {
			atMAC = atMACTmp
		}

		dataArrayBeforeMAC := eapAKAHdr + atRand + atAutn + atKdf + atKdfInput + atMAC
		eapPkt.Data = []byte(dataArrayBeforeMAC)
		encodedPktBeforeMAC := eapPkt.Encode()

		MacValue := CalculateAtMAC(K_aut, encodedPktBeforeMAC)
		atMAC = atMAC[:4] + string(MacValue)

		dataArrayAfterMAC := eapAKAHdr + atRand + atAutn + atKdf + atKdfInput + atMAC

		eapPkt.Data = []byte(dataArrayAfterMAC)
		encodedPktAfterMAC := eapPkt.Encode()
		responseBody.Var5gAuthData = base64.StdEncoding.EncodeToString(encodedPktAfterMAC)

		linksValue := models.LinksValueSchema{Href: putLink}
		responseBody.Links = make(map[string]models.LinksValueSchema)
		responseBody.Links["eap-session"] = linksValue
	}

	responseBody.AuthType = authInfoResult.AuthType

	return &responseBody, locationURI, nil
}

// func Auth5gAkaComfirmRequestProcedure(updateConfirmationData models.ConfirmationData,
//	ConfirmationDataResponseID string) (response *models.ConfirmationDataResponse,
//  problemDetails *models.ProblemDetails) {

func Auth5gAkaComfirmRequestProcedure(updateConfirmationData models.ConfirmationData,
	ConfirmationDataResponseID string,
) (*models.ConfirmationDataResponse, *models.ProblemDetails) {
	var responseBody models.ConfirmationDataResponse
	success := false
	responseBody.AuthResult = models.AuthResult_FAILURE

	if !ausf_context.CheckIfSuciSupiPairExists(ConfirmationDataResponseID) {
		logger.Auth5gAkaComfirmLog.Infof("supiSuciPair does not exist, confirmation failed (queried by %s)\n",
			ConfirmationDataResponseID)
		var problemDetails models.ProblemDetails
		problemDetails.Cause = "USER_NOT_FOUND"
		problemDetails.Status = http.StatusBadRequest
		return nil, &problemDetails
	}

	currentSupi := ausf_context.GetSupiFromSuciSupiMap(ConfirmationDataResponseID)
	if !ausf_context.CheckIfAusfUeContextExists(currentSupi) {
		logger.Auth5gAkaComfirmLog.Infof("SUPI does not exist, confirmation failed (queried by %s)\n", currentSupi)
		var problemDetails models.ProblemDetails
		problemDetails.Cause = "USER_NOT_FOUND"
		problemDetails.Status = http.StatusBadRequest
		return nil, &problemDetails
	}

	ausfCurrentContext := ausf_context.GetAusfUeContext(currentSupi)
	servingNetworkName := ausfCurrentContext.ServingNetworkName

	// Compare the received RES* with the stored XRES*
	logger.Auth5gAkaComfirmLog.Infof("res*: %x\nXres*: %x\n", updateConfirmationData.ResStar, ausfCurrentContext.XresStar)
	if strings.Compare(updateConfirmationData.ResStar, ausfCurrentContext.XresStar) == 0 {
		ausfCurrentContext.AuthStatus = models.AuthResult_SUCCESS
		responseBody.AuthResult = models.AuthResult_SUCCESS
		success = true
		logger.Auth5gAkaComfirmLog.Infoln("5G AKA confirmation succeeded")
		responseBody.Supi = currentSupi
		responseBody.Kseaf = ausfCurrentContext.Kseaf
	} else {
		ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		responseBody.AuthResult = models.AuthResult_FAILURE
		logConfirmFailureAndInformUDM(ConfirmationDataResponseID, models.AuthType__5_G_AKA, servingNetworkName,
			"5G AKA confirmation failed", ausfCurrentContext.UdmUeauUrl)
	}

	if sendErr := sendAuthResultToUDM(currentSupi, models.AuthType__5_G_AKA, success, servingNetworkName,
		ausfCurrentContext.UdmUeauUrl); sendErr != nil {
		logger.Auth5gAkaComfirmLog.Infoln(sendErr.Error())
		var problemDetails models.ProblemDetails
		problemDetails.Status = http.StatusInternalServerError
		problemDetails.Cause = "UPSTREAM_SERVER_ERROR"

		return nil, &problemDetails
	}

	return &responseBody, nil
}

// return response, problemDetails
func EapAuthComfirmRequestProcedure(updateEapSession models.EapSession, eapSessionID string) (*models.EapSession,
	*models.ProblemDetails,
) {
	var responseBody models.EapSession

	if !ausf_context.CheckIfSuciSupiPairExists(eapSessionID) {
		logger.EapAuthComfirmLog.Infoln("supiSuciPair does not exist, confirmation failed")
		var problemDetails models.ProblemDetails
		problemDetails.Cause = "USER_NOT_FOUND"
		return nil, &problemDetails
	}

	currentSupi := ausf_context.GetSupiFromSuciSupiMap(eapSessionID)
	if !ausf_context.CheckIfAusfUeContextExists(currentSupi) {
		logger.EapAuthComfirmLog.Infoln("SUPI does not exist, confirmation failed")
		var problemDetails models.ProblemDetails
		problemDetails.Cause = "USER_NOT_FOUND"
		return nil, &problemDetails
	}

	ausfCurrentContext := ausf_context.GetAusfUeContext(currentSupi)
	servingNetworkName := ausfCurrentContext.ServingNetworkName

	if ausfCurrentContext.AuthStatus == models.AuthResult_FAILURE {
		eapFailPkt := ConstructEapNoTypePkt(radius.EapCodeFailure, 0)
		responseBody.EapPayload = eapFailPkt
		responseBody.AuthResult = models.AuthResult_FAILURE
		return &responseBody, nil
	}

	var eapPayload []byte
	if eapPayloadTmp, err := base64.StdEncoding.DecodeString(updateEapSession.EapPayload); err != nil {
		logger.EapAuthComfirmLog.Warnf("EAP Payload decode failed: %+v", err)
	} else {
		eapPayload = eapPayloadTmp
	}

	eapGoPkt := gopacket.NewPacket(eapPayload, layers.LayerTypeEAP, gopacket.Default)
	eapLayer := eapGoPkt.Layer(layers.LayerTypeEAP)
	eapContent, _ := eapLayer.(*layers.EAP)
	eapOK := true
	var eapErrStr string

	if eapContent.Code != layers.EAPCodeResponse {
		eapOK = false
		eapErrStr = "eap packet code error"
	} else if eapContent.Type != ausf_context.EAP_AKA_PRIME_TYPENUM {
		eapOK = false
		eapErrStr = "eap packet type error"
	} else if decodeEapAkaPrimePkt, err := decodeEapAkaPrime(eapContent.Contents); err != nil {
		logger.EapAuthComfirmLog.Warnf("EAP-AKA' decode failed: %+v", err)
		eapOK = false
		eapErrStr = "eap packet error"
	} else {
		switch decodeEapAkaPrimePkt.Subtype {
		case ausf_context.AKA_CHALLENGE_SUBTYPE:
			K_autStr := ausfCurrentContext.K_aut
			var K_aut []byte
			if K_autTmp, err := hex.DecodeString(K_autStr); err != nil {
				logger.EapAuthComfirmLog.Warnf("K_aut decode error: %+v", err)
			} else {
				K_aut = K_autTmp
			}
			XMAC := CalculateAtMAC(K_aut, decodeEapAkaPrimePkt.MACInput)
			MAC := decodeEapAkaPrimePkt.Attributes[ausf_context.AT_MAC_ATTRIBUTE].Value
			XRES := ausfCurrentContext.XRES
			RES := hex.EncodeToString(decodeEapAkaPrimePkt.Attributes[ausf_context.AT_RES_ATTRIBUTE].Value)

			if !bytes.Equal(MAC, XMAC) {
				eapOK = false
				eapErrStr = "EAP-AKA' integrity check fail"
			} else if XRES == RES {
				logger.EapAuthComfirmLog.Infoln("Correct RES value, EAP-AKA' auth succeed")
				responseBody.KSeaf = ausfCurrentContext.Kseaf
				responseBody.Supi = currentSupi
				responseBody.AuthResult = models.AuthResult_SUCCESS
				eapSuccPkt := ConstructEapNoTypePkt(radius.EapCodeSuccess, eapContent.Id)
				responseBody.EapPayload = eapSuccPkt
				udmUrl := ausfCurrentContext.UdmUeauUrl
				if sendErr := sendAuthResultToUDM(
					eapSessionID,
					models.AuthType_EAP_AKA_PRIME,
					true,
					servingNetworkName,
					udmUrl); sendErr != nil {
					logger.EapAuthComfirmLog.Infoln(sendErr.Error())
					var problemDetails models.ProblemDetails
					problemDetails.Cause = "UPSTREAM_SERVER_ERROR"
					return nil, &problemDetails
				}
				ausfCurrentContext.AuthStatus = models.AuthResult_SUCCESS
			} else {
				eapOK = false
				eapErrStr = "Wrong RES value, EAP-AKA' auth failed"
			}
		case ausf_context.AKA_AUTHENTICATION_REJECT_SUBTYPE:
			ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		case ausf_context.AKA_SYNCHRONIZATION_FAILURE_SUBTYPE:
			logger.EapAuthComfirmLog.Warnf("EAP-AKA' synchronziation failure")
			if ausfCurrentContext.Resynced {
				eapOK = false
				eapErrStr = "2 consecutive Synch Failure, terminate authentication procedure"
			} else {
				var authInfo models.AuthenticationInfo
				AUTS := decodeEapAkaPrimePkt.Attributes[ausf_context.AT_AUTS_ATTRIBUTE].Value
				resynchronizationInfo := &models.ResynchronizationInfo{
					Auts: hex.EncodeToString(AUTS[:]),
				}
				authInfo.SupiOrSuci = eapSessionID
				authInfo.ServingNetworkName = servingNetworkName
				authInfo.ResynchronizationInfo = resynchronizationInfo
				response, _, problemDetails := UeAuthPostRequestProcedure(authInfo)
				if problemDetails != nil {
					return nil, problemDetails
				}
				ausfCurrentContext.Resynced = true

				responseBody.EapPayload = response.Var5gAuthData.(string)
				responseBody.Links = response.Links
				responseBody.AuthResult = models.AuthResult_ONGOING
			}
		case ausf_context.AKA_NOTIFICATION_SUBTYPE:
			ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		case ausf_context.AKA_CLIENT_ERROR_SUBTYPE:
			logger.EapAuthComfirmLog.Warnf("EAP-AKA' failure: receive client-error")
			ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		default:
			ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		}
	}

	if !eapOK {
		logger.EapAuthComfirmLog.Warnf("EAP-AKA' failure: %s", eapErrStr)
		if sendErr := sendAuthResultToUDM(eapSessionID, models.AuthType_EAP_AKA_PRIME, false, servingNetworkName,
			ausfCurrentContext.UdmUeauUrl); sendErr != nil {
			logger.EapAuthComfirmLog.Infoln(sendErr.Error())
			var problemDetails models.ProblemDetails
			problemDetails.Status = http.StatusInternalServerError
			problemDetails.Cause = "UPSTREAM_SERVER_ERROR"

			return nil, &problemDetails
		}

		ausfCurrentContext.AuthStatus = models.AuthResult_FAILURE
		responseBody.AuthResult = models.AuthResult_ONGOING
		failEapAkaNoti := ConstructFailEapAkaNotification(eapContent.Id)
		responseBody.EapPayload = failEapAkaNoti
		self := ausf_context.GetSelf()
		linkUrl := self.Url + "/nausf-auth/v1/ue-authentications/" + eapSessionID + "/eap-session"
		linksValue := models.LinksValueSchema{Href: linkUrl}
		responseBody.Links = make(map[string]models.LinksValueSchema)
		responseBody.Links["eap-session"] = linksValue
	} else if ausfCurrentContext.AuthStatus == models.AuthResult_FAILURE {
		if sendErr := sendAuthResultToUDM(eapSessionID, models.AuthType_EAP_AKA_PRIME, false, servingNetworkName,
			ausfCurrentContext.UdmUeauUrl); sendErr != nil {
			logger.EapAuthComfirmLog.Infoln(sendErr.Error())
			var problemDetails models.ProblemDetails
			problemDetails.Status = http.StatusInternalServerError
			problemDetails.Cause = "UPSTREAM_SERVER_ERROR"

			return nil, &problemDetails
		}

		eapFailPkt := ConstructEapNoTypePkt(radius.EapCodeFailure, eapPayload[1])
		responseBody.EapPayload = eapFailPkt
		responseBody.AuthResult = models.AuthResult_FAILURE
	}

	return &responseBody, nil
}
