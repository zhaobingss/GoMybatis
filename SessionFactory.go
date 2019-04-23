package GoMybatis

import "sync"

type SessionFactory struct {
	Engine     SessionEngine
	//SessionMap map[string]Session
	SessionMap *sync.Map
}

func (it SessionFactory) New(Engine SessionEngine) SessionFactory {
	it.Engine = Engine
	it.SessionMap = &sync.Map{}
	return it
}

func (it *SessionFactory) NewSession(mapperName string, sessionType SessionType, config *TransationRMClientConfig) Session {
	if it.SessionMap == nil || it.Engine == nil {
		panic("[GoMybatis] SessionFactory not init! you must call method SessionFactory.New(*)")
	}
	var newSession Session
	var err error
	switch sessionType {
	case SessionType_Default:
		var session, err = it.Engine.NewSession(mapperName)
		if err != nil {
			panic(err)
		}
		var factorySession = SessionFactorySession{
			Session: session,
			Factory: it,
		}
		newSession = Session(&factorySession)
		break
	case SessionType_Local:
		newSession, err = it.Engine.NewSession(mapperName)
		if err != nil {
			panic(err)
		}
		break
	case SessionType_TransationRM:
		if config == nil {
			panic("[GoMybatis] SessionFactory can not create TransationRMSession,config *TransationRMClientConfig is nil!")
		}
		var transationRMSession = TransationRMSession{}.New(mapperName, config.TransactionId, &TransationRMClient{
			RetryTime: config.RetryTime,
			Addr:      config.Addr,
		}, config.Status)
		newSession = Session(*transationRMSession)
		break
	default:
		panic("[GoMybatis] newSession() must have a SessionType!")
	}
	//it.SessionMap[newSession.Id()] = newSession
	it.SessionMap.Store(newSession.Id(), newSession)
	return newSession
}

func (it *SessionFactory) GetSession(id string) Session {
	s,_ := it.SessionMap.Load(id)
	return s.(Session)
	//return it.SessionMap[id]
}

func (it *SessionFactory) SetSession(id string, session Session) {
	it.SessionMap.Store(id, session)
	//it.SessionMap[id] = session
}

func (it *SessionFactory) Close(id string) {
	if it.SessionMap == nil {
		return
	}
	s,_ := it.SessionMap.Load(id)
	//var s = it.SessionMap[id]
	if s != nil {
		//s.Close()
		//it.SessionMap[id] = nil
		s.(Session).Close()
		it.SessionMap.Store(id, nil)
	}
}

func (it *SessionFactory) CloseAll(id string) {
	if it.SessionMap == nil {
		return
	}
	//for _, v := range it.SessionMap {
	//	if v != nil {
	//		v.Close()
	//		it.SessionMap[id] = nil
	//	}
	//}
	it.SessionMap.Range(func(key, value interface{}) bool {
		value.(Session).Close()
		it.SessionMap.Store(key, nil)
		return true
	})
}
