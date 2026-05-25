
package sdkInit

import (
	"fmt"
	"time"
	mb "github.com/hyperledger/fabric-protos-go/msp"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/channel"
	mspclient "github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/resmgmt"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/status"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	lcpackager "github.com/hyperledger/fabric-sdk-go/pkg/fab/ccpackager/lifecycle"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/third_party/github.com/hyperledger/fabric/common/policydsl"
	"strings"
)

func Setup(configFile string, info *SdkEnvInfo) (*fabsdk.FabricSDK, error) {
	// Create SDK setup for the integration tests
	var err error
	sdk, err := fabsdk.New(config.FromFile(configFile))
	if err != nil {
		return nil, err
	}

	// 为组织获得Client句柄和Context信息
	for _, org := range info.Orgs {
		org.orgMspClient, err = mspclient.New(sdk.Context(), mspclient.WithOrg(org.OrgName))
		if err != nil {
			return nil, err
		}
		orgContext := sdk.Context(fabsdk.WithUser(org.OrgAdminUser), fabsdk.WithOrg(org.OrgName))
		org.OrgAdminClientContext = &orgContext

		// New returns a resource management client instance.
		resMgmtClient, err := resmgmt.New(orgContext)
		if err != nil {
			return nil, fmt.Errorf("지정된 리소스 관리 클라이언트 Context를 기반으로 채널 관리 클라이언트 생성 실패: %v", err)
		}
		org.OrgResMgmt = resMgmtClient
	}

	// 为Orderer获得Context信息
	ordererClientContext := sdk.Context(fabsdk.WithUser(info.OrdererAdminUser), fabsdk.WithOrg(info.OrdererOrgName))
	info.OrdererClientContext = &ordererClientContext
	return sdk, nil
}

func CreateAndJoinChannel(info *SdkEnvInfo) error {
    fmt.Println(">> 检查通道是否已存在/已加入...")

    // 等容器都起来（orderer/peer 刚启动时会拒绝，给一点缓冲）
    time.Sleep(2 * time.Second)

    // 先判断通道是否已存在（通过查询 org1 的 peer）
    exists, err := channelExists(info)
    if err != nil {
        return fmt.Errorf("预检查通道存在性失败: %v", err)
    }

    createdNow := false
    if !exists {
        fmt.Println(">> 通道不存在，准备创建通道...")
        // 收集签名身份
        signIds := []msp.SigningIdentity{}
        for _, org := range info.Orgs {
            sid, err := org.orgMspClient.GetSigningIdentity(org.OrgAdminUser)
            if err != nil {
                return fmt.Errorf("GetSigningIdentity error: %v", err)
            }
            signIds = append(signIds, sid)
        }
        // 创建通道（只做 SaveChannel，不做锚点）
        if err := createChannel(signIds, info); err != nil {
            return fmt.Errorf("Create channel error: %v", err)
        }
        createdNow = true
        fmt.Println(">> 通道创建完成")
        // 给 orderer 一点时间出块
        time.Sleep(2 * time.Second)
    } else {
        fmt.Println(">> 通道已存在，跳过创建")
    }

    // 逐个 org 确保加入通道（已加入就跳过）
    fmt.Println(">> 确保各组织已加入通道...")
    for _, org := range info.Orgs {
        joined, err := peerJoinedChannel(org, info.ChannelID)
        if err != nil {
            return fmt.Errorf("检查组织 %s 是否已加入通道失败: %v", org.OrgName, err)
        }
        if joined {
            fmt.Printf("   - %s 已在通道，跳过 JoinChannel\n", org.OrgName)
            continue
        }
        if err := org.OrgResMgmt.JoinChannel(info.ChannelID,
            resmgmt.WithRetry(retry.DefaultResMgmtOpts),
            resmgmt.WithOrdererEndpoint("orderer.example.com")); err != nil {
            return fmt.Errorf("%s peers failed to JoinChannel: %v", org.OrgName, err)
        }
        fmt.Printf("   - %s 加入通道完成\n", org.OrgName)
    }
    fmt.Println(">> 组织加入通道检查完成")

    // 锚点更新：仅在“本次新建通道”的时候做；之后重跑跳过
    if createdNow {
        fmt.Println(">> 首次创建通道，更新锚节点配置...")
        // 用 org 顺序与 signIds 对应关系：这里重新拿各 org 的 SigningIdentity
        for _, org := range info.Orgs {
            sid, err := org.orgMspClient.GetSigningIdentity(org.OrgAdminUser)
            if err != nil {
                return fmt.Errorf("GetSigningIdentity error: %v", err)
            }
            req := resmgmt.SaveChannelRequest{
                ChannelID:         info.ChannelID,
                ChannelConfigPath: org.OrgAnchorFile,
                SigningIdentities: []msp.SigningIdentity{sid},
            }
            if _, err = org.OrgResMgmt.SaveChannel(req,
                resmgmt.WithRetry(retry.DefaultResMgmtOpts),
                resmgmt.WithOrdererEndpoint("orderer.example.com")); err != nil {
                return fmt.Errorf("更新锚节点失败（%s）: %v", org.OrgName, err)
            }
        }
        fmt.Println(">> 锚节点更新完成")
    } else {
        fmt.Println(">> 非首次启动，跳过锚节点更新")
    }

    return nil
}

func createChannel(signIDs []msp.SigningIdentity, info *SdkEnvInfo) error {
    chMgmtClient, err := resmgmt.New(*info.OrdererClientContext)
    if err != nil {
        return fmt.Errorf("Channel management client create error: %v", err)
    }

    req := resmgmt.SaveChannelRequest{
        ChannelID:         info.ChannelID,
        ChannelConfigPath: info.ChannelConfig,
        SigningIdentities: signIDs,
    }
    if _, err := chMgmtClient.SaveChannel(req,
        resmgmt.WithRetry(retry.DefaultResMgmtOpts),
        resmgmt.WithOrdererEndpoint("orderer.example.com")); err != nil {
        return fmt.Errorf("SaveChannel error: %v", err)
    }
    return nil
}

func CreateCCLifecycle(info *SdkEnvInfo, _ int64, upgrade bool, sdk *fabsdk.FabricSDK) error {
    if len(info.Orgs) == 0 {
        return fmt.Errorf("the number of organization should not be zero.")
    }

    // 先看看是否已经有提交过的定义
    committedSeq, hasCommitted, err := getCommittedSequence(info.Orgs[0], info.ChannelID, info.ChaincodeID)
    if err != nil {
        return fmt.Errorf("query committed cc error: %v", err)
    }

    if hasCommitted && !upgrade {
        fmt.Printf(">> 链码 %s 已提交（sequence=%d），非升级模式，跳过审批/提交/初始化\n", info.ChaincodeID, committedSeq)
        return nil
    }

    // 计算这次应使用的 sequence
    seqToUse := int64(1)
    if hasCommitted {
        seqToUse = committedSeq + 1
        fmt.Printf(">> 检测到已有提交 sequence=%d，本次为升级，sequence 设为 %d\n", committedSeq, seqToUse)
    } else {
        fmt.Println(">> 首次部署链码，sequence=1")
    }

    // Package cc
    fmt.Println(">> 체인코드 패키징 시작......")
    label, ccPkg, err := packageCC(info.ChaincodeID, info.ChaincodeVersion, info.ChaincodePath)
    if err != nil {
        return fmt.Errorf("pakcagecc error: %v", err)
    }
    packageID := lcpackager.ComputePackageID(label, ccPkg)
    fmt.Println(">> 체인코드 패키징 성공")

    // Install cc（安装已做幂等检查）
    fmt.Println(">> 체인코드 설치 시작......")
    if err := installCC(label, ccPkg, info.Orgs); err != nil {
        return fmt.Errorf("installCC error: %v", err)
    }
    if err := getInstalledCCPackage(packageID, info.Orgs[0]); err != nil {
        return fmt.Errorf("getInstalledCCPackage error: %v", err)
    }
    if err := queryInstalled(packageID, info.Orgs[0]); err != nil {
        return fmt.Errorf("queryInstalled error: %v", err)
    }
    fmt.Println(">> 체인코드 설치 성공")

    // Approve cc（用 seqToUse）
    fmt.Println(">> 조직이 스마트 컨트랙트 정의를 승인 중......")
    if err := approveCC(packageID, info.ChaincodeID, info.ChaincodeVersion, seqToUse, info.ChannelID, info.Orgs, info.OrdererEndpoint); err != nil {
        return fmt.Errorf("approveCC error: %v", err)
    }
    if err := queryApprovedCC(info.ChaincodeID, seqToUse, info.ChannelID, info.Orgs); err != nil {
        return fmt.Errorf("queryApprovedCC error: %v", err)
    }
    fmt.Println(">> 조직이 스마트 컨트랙트 정의 승인 완료")

    // Check commit readiness
    fmt.Println(">> 스마트 컨트랙트가 준비되었는지 확인 중......")
    if err := checkCCCommitReadiness(packageID, info.ChaincodeID, info.ChaincodeVersion, seqToUse, info.ChannelID, info.Orgs); err != nil {
        return fmt.Errorf("checkCCCommitReadiness error: %v", err)
    }
    fmt.Println(">> 스마트 컨트랙트가 준비 완료")

    // Commit
    fmt.Println(">> 스마트 컨트랙트 정의를 제출 중......")
    if err := commitCC(info.ChaincodeID, info.ChaincodeVersion, seqToUse, info.ChannelID, info.Orgs, info.OrdererEndpoint); err != nil {
        return fmt.Errorf("commitCC error: %v", err)
    }
    if err := queryCommittedCC(info.ChaincodeID, info.ChannelID, seqToUse, info.Orgs); err != nil {
        return fmt.Errorf("queryCommittedCC error: %v", err)
    }
    fmt.Println(">> 스마트 컨트랙트 정의 제출 완료")

    // Init：仅首次部署或升级时执行（InitRequired=true 的定义，每次升级后仍需再 init 一次）
    if !hasCommitted || upgrade {
        fmt.Println(">> 스마트 컨트랙트 초기화 함수 호출 중......")
        if err := initCC(info.ChaincodeID, true, info.ChannelID, info.Orgs[0], sdk); err != nil {
            return fmt.Errorf("initCC error: %v", err)
        }
        fmt.Println(">> 스마트 컨트랙트 초기화 완료")
    } else {
        fmt.Println(">> 非升级且已部署过，跳过 init")
    }

    return nil
}

func packageCC(ccName, ccVersion, ccpath string) (string, []byte, error) {
	label := ccName + "_" + ccVersion
	desc := &lcpackager.Descriptor{
		Path:  ccpath,
		Type:  pb.ChaincodeSpec_GOLANG,
		Label: label,
	}
	ccPkg, err := lcpackager.NewCCPackage(desc)
	if err != nil {
		return "", nil, fmt.Errorf("Package chaincode source error: %v", err)
	}
	return desc.Label, ccPkg, nil
}

func installCC(label string, ccPkg []byte, orgs []*OrgInfo) error {
	installCCReq := resmgmt.LifecycleInstallCCRequest{
		Label:   label,
		Package: ccPkg,
	}

	packageID := lcpackager.ComputePackageID(installCCReq.Label, installCCReq.Package)
	for _, org := range orgs {
		orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, org.OrgPeerNum)
		if err != nil {
			fmt.Errorf("DiscoverLocalPeers error: %v", err)
		}
		if flag, _ := checkInstalled(packageID, orgPeers[0], org.OrgResMgmt); flag == false {
			if _, err := org.OrgResMgmt.LifecycleInstallCC(installCCReq, resmgmt.WithTargets(orgPeers...), resmgmt.WithRetry(retry.DefaultResMgmtOpts)); err != nil {
				return fmt.Errorf("LifecycleInstallCC error: %v", err)
			}
		}
	}
	return nil
}

func getInstalledCCPackage(packageID string, org *OrgInfo) error {
	// use org1
	orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, 1)
	if err != nil {
		return fmt.Errorf("DiscoverLocalPeers error: %v", err)
	}

	if _, err := org.OrgResMgmt.LifecycleGetInstalledCCPackage(packageID, resmgmt.WithTargets([]fab.Peer{orgPeers[0]}...)); err != nil {
		return fmt.Errorf("LifecycleGetInstalledCCPackage error: %v", err)
	}
	return nil
}

func queryInstalled(packageID string, org *OrgInfo) error {
	orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, 1)
	if err != nil {
		return fmt.Errorf("DiscoverLocalPeers error: %v", err)
	}
	resp1, err := org.OrgResMgmt.LifecycleQueryInstalledCC(resmgmt.WithTargets([]fab.Peer{orgPeers[0]}...))
	if err != nil {
		return fmt.Errorf("LifecycleQueryInstalledCC error: %v", err)
	}
	packageID1 := ""
	for _, t := range resp1 {
		if t.PackageID == packageID {
			packageID1 = t.PackageID
		}
	}
	if !strings.EqualFold(packageID, packageID1) {
		return fmt.Errorf("check package id error")
	}
	return nil
}

func checkInstalled(packageID string, peer fab.Peer, client *resmgmt.Client) (bool, error) {
	flag := false
	resp1, err := client.LifecycleQueryInstalledCC(resmgmt.WithTargets(peer))
	if err != nil {
		return flag, fmt.Errorf("LifecycleQueryInstalledCC error: %v", err)
	}
	for _, t := range resp1 {
		if t.PackageID == packageID {
			flag = true
		}
	}
	return flag, nil
}

func approveCC(packageID string, ccName, ccVersion string, sequence int64, channelID string, orgs []*OrgInfo, ordererEndpoint string) error {
    mspIDs := []string{}
    for _, org := range orgs {
        mspIDs = append(mspIDs, org.OrgMspId)
    }
    ccPolicy := policydsl.SignedByNOutOfGivenRole(int32(len(mspIDs)), mb.MSPRole_MEMBER, mspIDs)
    approveCCReq := resmgmt.LifecycleApproveCCRequest{
        Name:              ccName,
        Version:           ccVersion,
        PackageID:         packageID,
        Sequence:          sequence,
        EndorsementPlugin: "escc",
        ValidationPlugin:  "vscc",
        SignaturePolicy:   ccPolicy,
        InitRequired:      true,
    }

    for _, org := range orgs{
        orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, org.OrgPeerNum)
        fmt.Printf(">>> chaincode approved by %s peers:\n", org.OrgName)
        for _, p := range orgPeers {
            fmt.Printf("    %s\n", p.URL())
        }
        if err != nil {
            return fmt.Errorf("DiscoverLocalPeers error: %v", err)
        }
        if _, err := org.OrgResMgmt.LifecycleApproveCC(
            channelID, approveCCReq,
            resmgmt.WithTargets(orgPeers...),
            resmgmt.WithOrdererEndpoint(ordererEndpoint),
            resmgmt.WithRetry(retry.DefaultResMgmtOpts),
        ); err != nil {
            return fmt.Errorf("LifecycleApproveCC error: %v", err) // ← 这里要 return
        }
    }
    return nil
}

func queryApprovedCC(ccName string, sequence int64, channelID string, orgs []*OrgInfo) error {
	queryApprovedCCReq := resmgmt.LifecycleQueryApprovedCCRequest{
		Name:     ccName,
		Sequence: sequence,
	}

	for _, org := range orgs{
		orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, org.OrgPeerNum)
		if err!=nil{
			return fmt.Errorf("DiscoverLocalPeers error: %v", err)
		}
		// Query approve cc
		for _, p := range orgPeers {
			resp, err := retry.NewInvoker(retry.New(retry.TestRetryOpts)).Invoke(
				func() (interface{}, error) {
					resp1, err := org.OrgResMgmt.LifecycleQueryApprovedCC(channelID, queryApprovedCCReq, resmgmt.WithTargets(p))
					if err != nil {
						return nil, status.New(status.TestStatus, status.GenericTransient.ToInt32(), fmt.Sprintf("LifecycleQueryApprovedCC returned error: %v", err), nil)
					}
					return resp1, err
				},
			)
			if err != nil {
				return fmt.Errorf("Org %s Peer %s NewInvoker error: %v", org.OrgName, p.URL(), err)
			}
			if resp==nil{
				return fmt.Errorf("Org %s Peer %s Got nil invoker", org.OrgName, p.URL())
			}
		}
	}
	return nil
}

func checkCCCommitReadiness(packageID string, ccName, ccVersion string, sequence int64, channelID string, orgs []*OrgInfo) error {
	mspIds := []string{}
	for _, org := range orgs {
		mspIds = append(mspIds, org.OrgMspId)
	}
	ccPolicy := policydsl.SignedByNOutOfGivenRole(int32(len(mspIds)), mb.MSPRole_MEMBER, mspIds)
	req := resmgmt.LifecycleCheckCCCommitReadinessRequest{
		Name:              ccName,
		Version:           ccVersion,
		//PackageID:         packageID,
		EndorsementPlugin: "escc",
		ValidationPlugin:  "vscc",
		SignaturePolicy:   ccPolicy,
		Sequence:          sequence,
		InitRequired:      true,
	}
	for _, org := range orgs{
		orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, org.OrgPeerNum)
		if err!=nil{
			fmt.Errorf("DiscoverLocalPeers error: %v", err)
		}
		for _, p := range orgPeers {
			resp, err := retry.NewInvoker(retry.New(retry.TestRetryOpts)).Invoke(
				func() (interface{}, error) {
					resp1, err := org.OrgResMgmt.LifecycleCheckCCCommitReadiness(channelID, req, resmgmt.WithTargets(p))
					fmt.Printf("LifecycleCheckCCCommitReadiness cc = %v, = %v\n", ccName, resp1)
					if err != nil {
						return nil, status.New(status.TestStatus, status.GenericTransient.ToInt32(), fmt.Sprintf("LifecycleCheckCCCommitReadiness returned error: %v", err), nil)
					}
					flag := true
					for _, r := range resp1.Approvals {
						flag = flag && r
					}
					if !flag {
						return nil, status.New(status.TestStatus, status.GenericTransient.ToInt32(), fmt.Sprintf("LifecycleCheckCCCommitReadiness returned : %v", resp1), nil)
					}
					return resp1, err
				},
			)
			if err != nil {
				return fmt.Errorf("NewInvoker error: %v", err)
			}
			if resp==nil{
				return fmt.Errorf("Got nill invoker response")
			}
		}
	}

	return nil
}

func commitCC(ccName, ccVersion string, sequence int64, channelID string, orgs []*OrgInfo, ordererEndpoint string) error{
	mspIDs := []string{}
	for _, org := range orgs {
		mspIDs = append(mspIDs, org.OrgMspId)
	}
	ccPolicy := policydsl.SignedByNOutOfGivenRole(int32(len(mspIDs)), mb.MSPRole_MEMBER, mspIDs)

	req := resmgmt.LifecycleCommitCCRequest{
		Name:              ccName,
		Version:           ccVersion,
		Sequence:          sequence,
		EndorsementPlugin: "escc",
		ValidationPlugin:  "vscc",
		SignaturePolicy:   ccPolicy,
		InitRequired:      true,
	}
	_, err := orgs[0].OrgResMgmt.LifecycleCommitCC(channelID, req, resmgmt.WithOrdererEndpoint(ordererEndpoint), resmgmt.WithRetry(retry.DefaultResMgmtOpts))
	if err != nil {
		return fmt.Errorf("LifecycleCommitCC error: %v", err)
	}
	return nil
}

func queryCommittedCC( ccName string, channelID string, sequence int64, orgs []*OrgInfo) error {
	req := resmgmt.LifecycleQueryCommittedCCRequest{
		Name: ccName,
	}

	for _, org := range orgs {
		orgPeers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, org.OrgPeerNum)
		if err!=nil{
			return fmt.Errorf("DiscoverLocalPeers error: %v", err)
		}
		for _, p := range orgPeers {
			resp, err := retry.NewInvoker(retry.New(retry.TestRetryOpts)).Invoke(
				func() (interface{}, error) {
					resp1, err := org.OrgResMgmt.LifecycleQueryCommittedCC(channelID, req, resmgmt.WithTargets(p))
					if err != nil {
						return nil, status.New(status.TestStatus, status.GenericTransient.ToInt32(), fmt.Sprintf("LifecycleQueryCommittedCC returned error: %v", err), nil)
					}
					flag := false
					for _, r := range resp1 {
						if r.Name == ccName && r.Sequence == sequence {
							flag = true
							break
						}
					}
					if !flag {
						return nil, status.New(status.TestStatus, status.GenericTransient.ToInt32(), fmt.Sprintf("LifecycleQueryCommittedCC returned : %v", resp1), nil)
					}
					return resp1, err
				},
			)
			if err != nil {
				return  fmt.Errorf("NewInvoker error: %v", err)
			}
			if resp==nil{
				return fmt.Errorf("Got nil invoker response")
			}
		}
	}
	return nil
}

func initCC(ccName string, upgrade bool, channelID string, org *OrgInfo, sdk *fabsdk.FabricSDK) error {
	//prepare channel client context using client context
	clientChannelContext := sdk.ChannelContext(channelID, fabsdk.WithUser(org.OrgUser), fabsdk.WithOrg(org.OrgName))
	// Channel client is used to query and execute transactions (Org1 is default org)
	client, err := channel.New(clientChannelContext)
	if err != nil {
		return fmt.Errorf("Failed to create new channel client: %s", err)
	}

	// init
	_, err = client.Execute(channel.Request{ChaincodeID: ccName, Fcn: "init", Args: nil, IsInit: true},
		channel.WithRetry(retry.DefaultChannelOpts))
	if err != nil {
		return fmt.Errorf("Failed to init: %s", err)
	}
	return nil
}

// 判断某个 org 的任意一个 Peer 是否已经在指定通道
func peerJoinedChannel(org *OrgInfo, channelID string) (bool, error) {
    peers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, 1)
    if err != nil {
        return false, fmt.Errorf("DiscoverLocalPeers error: %v", err)
    }
    if len(peers) == 0 {
        return false, fmt.Errorf("no peers discovered for org %s", org.OrgName)
    }
    // 只要看第一个 Peer 的已加入通道列表
    resp, err := org.OrgResMgmt.QueryChannels(resmgmt.WithTargets(peers[0]))
    if err != nil {
        return false, fmt.Errorf("QueryChannels error: %v", err)
    }
    for _, ch := range resp.Channels {
        if ch.ChannelId == channelID {
            return true, nil
        }
    }
    return false, nil
}

// 至少用第一个 org 的 Peer 来判断“通道是否已存在于网络”
func channelExists(info *SdkEnvInfo) (bool, error) {
    if len(info.Orgs) == 0 {
        return false, fmt.Errorf("no orgs configured")
    }
    return peerJoinedChannel(info.Orgs[0], info.ChannelID)
}

// 查询某 org 的一个 Peer 上已提交的链码定义 sequence；
// 若未提交过，返回 (0, false, nil)
func getCommittedSequence(org *OrgInfo, channelID, ccName string) (int64, bool, error) {
    peers, err := DiscoverLocalPeers(*org.OrgAdminClientContext, 1)
    if err != nil {
        return 0, false, fmt.Errorf("DiscoverLocalPeers error: %v", err)
    }
    if len(peers) == 0 {
        return 0, false, fmt.Errorf("no peers discovered for org %s", org.OrgName)
    }

    req := resmgmt.LifecycleQueryCommittedCCRequest{Name: ccName}
    resp, err := org.OrgResMgmt.LifecycleQueryCommittedCC(channelID, req, resmgmt.WithTargets(peers[0]))
    if err != nil {
        // 通常这里表示还没有提交过定义（或还没同步到这个 peer），按“未提交”处理
        return 0, false, nil
    }
    for _, r := range resp {
        if r.Name == ccName {
            return int64(r.Sequence), true, nil
        }
    }
    return 0, false, nil
}

