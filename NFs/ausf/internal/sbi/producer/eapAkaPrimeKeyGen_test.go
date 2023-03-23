package producer

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/free5gc/util/ueauth"
)

type testEapAkaPrimeCase struct {
	Identity    string `json:"Identity"`
	NetworkName string `json:"Networkname"`
	RAND        string `json:"RAND"`
	AUTN        string `json:"AUTN"`
	IK          string `json:"IK"`
	CK          string `json:"CK"`
	RES         string `json:"RES"`
	CKPrime     string `json:"CKPrime"`
	IKPrime     string `json:"IKPrime"`
	K_encr      string `json:"K_encr"`
	K_aut       string `json:"K_aut"`
	K_re        string `json:"K_re"`
	MSK         string `json:"MSK"`
	EMSK        string `json:"EMSK"`
}

func EapAkaPrimeKeyGenAll(data testEapAkaPrimeCase) ([]byte, []byte, []byte, []byte, []byte, []byte, []byte) {
	var CK, IK, AUTN []byte
	if CKtmp, err := hex.DecodeString(data.CK); err != nil {
		fmt.Println(err)
	} else {
		CK = CKtmp
	}
	if IKtmp, err := hex.DecodeString(data.IK); err != nil {
		fmt.Println(err)
	} else {
		IK = IKtmp
	}
	if AUTNtmp, err := hex.DecodeString(data.AUTN); err != nil {
		fmt.Println(err)
	} else {
		AUTN = AUTNtmp
	}
	SQNxorAK := AUTN[:6]
	key := append(CK, IK...)
	FC := ueauth.FC_FOR_CK_PRIME_IK_PRIME_DERIVATION
	P0 := []byte(data.NetworkName)
	P1 := SQNxorAK

	// Generate CK' IK'
	kdfVal, err := ueauth.GetKDFValue(key, FC, P0, ueauth.KDFLen(P0), P1, ueauth.KDFLen(P1))
	if err != nil {
		fmt.Println(err)
	}
	CKPrime := kdfVal[:len(kdfVal)/2]
	IKPrime := kdfVal[len(kdfVal)/2:]
	CKPrimeHex := hex.EncodeToString(CKPrime)
	IKPrimeHex := hex.EncodeToString(IKPrime)

	// Generate K_encr K_aut K_re MSK EMSK
	K_encr, K_aut, K_re, MSK, EMSK := eapAkaPrimePrf(IKPrimeHex, CKPrimeHex, data.Identity)
	return CKPrime, IKPrime, K_encr, K_aut, K_re, MSK, EMSK
}

func TestEapAkaPrimeKeyGen(t *testing.T) {
	// From RFC 5448 Appendix C
	testCases := []testEapAkaPrimeCase{
		{
			"0555444333222111",
			"WLAN",
			"81e92b6c0ee0e12ebceba8d92a99dfa5",
			"bb52e91c747ac3ab2a5c23d15ee351d5",
			"9744871ad32bf9bbd1dd5ce54e3e2e5a",
			"5349fbe098649f948f5d2e973a81c00f",
			"28d7b0f2a2ec3de5",
			"0093962d0dd84aa5684b045c9edffa04",
			"ccfc230ca74fcc96c0a5d61164f5a76c",
			"766fa0a6c317174b812d52fbcd11a179",
			"0842ea722ff6835bfa2032499fc3ec23c2f0e388b4f07543ffc677f1696d71ea",
			"cf83aa8bc7e0aced892acc98e76a9b2095b558c7795c7094715cb3393aa7d17a",
			"67c42d9aa56c1b79e295e3459fc3d187d42be0bf818d3070e362c5e967a4d544" +
				"e8ecfe19358ab3039aff03b7c930588c055babee58a02650b067ec4e9347c75a",
			"f861703cd775590e16c7679ea3874ada866311de290764d760cf76df647ea01c" +
				"313f69924bdd7650ca9bac141ea075c4ef9e8029c0e290cdbad5638b63bc23fb",
		},
		{
			"0555444333222111",
			"HRPD",
			"81e92b6c0ee0e12ebceba8d92a99dfa5",
			"bb52e91c747ac3ab2a5c23d15ee351d5",
			"9744871ad32bf9bbd1dd5ce54e3e2e5a",
			"5349fbe098649f948f5d2e973a81c00f",
			"28d7b0f2a2ec3de5",
			"3820f0277fa5f77732b1fb1d90c1a0da",
			"db94a0ab557ef6c9ab48619ca05b9a9f",
			"05ad73ac915fce89ac77e1520d82187b",
			"5b4acaef62c6ebb8882b2f3d534c4b35277337a00184f20ff25d224c04be2afd",
			"3f90bf5c6e5ef325ff04eb5ef6539fa8cca8398194fbd00be425b3f40dba10ac",
			"87b321570117cd6c95ab6c436fb5073ff15cf85505d2bc5bb7355fc21ea8a757" +
				"57e8f86a2b138002e05752913bb43b82f868a96117e91a2d95f526677d572900",
			"c891d5f20f148a1007553e2dea555c9cb672e9675f4a66b4bafa027379f93aee" +
				"539a5979d0a0042b9d2ae28bed3b17a31dc8ab75072b80bd0c1da612466e402c",
		},
		{
			"0555444333222111",
			"WLAN",
			"e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0",
			"a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0",
			"b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
			"c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0",
			"d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0",
			"cd4c8e5c68f57dd1d7d7dfd0c538e577",
			"3ece6b705dbbf7dfc459a11280c65524",
			"897d302fa2847416488c28e20dcb7be4",
			"c40700e7722483ae3dc7139eb0b88bb558cb3081eccd057f9207d1286ee7dd53",
			"0a591a22dd8b5b1cf29e3d508c91dbbdb4aee23051892c42b6a2de66ea504473",
			"9f7dca9e37bb22029ed986e7cd09d4a70d1ac76d95535c5cac40a7504699bb89" +
				"61a29ef6f3e90f183de5861ad1bedc81ce9916391b401aa006c98785a5756df7",
			"724de00bdb9e568187be3fe746114557d5018779537ee37f4d3c6c738cb97b9d" +
				"c651bc19bfadc344ffe2b52ca78bd8316b51dacc5f2b1440cb9515521cc7ba23",
		},
		{
			"0555444333222111",
			"WLAN",
			"e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0",
			"a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0",
			"b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
			"c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0",
			"d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0",
			"cd4c8e5c68f57dd1d7d7dfd0c538e577",
			"3ece6b705dbbf7dfc459a11280c65524",
			"897d302fa2847416488c28e20dcb7be4",
			"c40700e7722483ae3dc7139eb0b88bb558cb3081eccd057f9207d1286ee7dd53",
			"0a591a22dd8b5b1cf29e3d508c91dbbdb4aee23051892c42b6a2de66ea504473",
			"9f7dca9e37bb22029ed986e7cd09d4a70d1ac76d95535c5cac40a7504699bb89" +
				"61a29ef6f3e90f183de5861ad1bedc81ce9916391b401aa006c98785a5756df7",
			"724de00bdb9e568187be3fe746114557d5018779537ee37f4d3c6c738cb97b9d" +
				"c651bc19bfadc344ffe2b52ca78bd8316b51dacc5f2b1440cb9515521cc7ba23",
		},
	}

	for idx, testData := range testCases {
		fmt.Printf("Case %d\n", idx+1)
		CKPrime, IKPrime, K_encr, K_aut, K_re, MSK, EMSK := EapAkaPrimeKeyGenAll(testData)
		assert.True(t, testData.IKPrime == hex.EncodeToString(IKPrime))
		assert.True(t, testData.CKPrime == hex.EncodeToString(CKPrime))
		assert.True(t, testData.K_encr == hex.EncodeToString(K_encr))
		assert.True(t, testData.K_aut == hex.EncodeToString(K_aut))
		assert.True(t, testData.K_re == hex.EncodeToString(K_re))
		assert.True(t, testData.MSK == hex.EncodeToString(MSK))
		assert.True(t, testData.EMSK == hex.EncodeToString(EMSK))
	}
}
